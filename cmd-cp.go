/*
 * Mini Copy, (C) 2014,2015 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"io"

	"github.com/cheggaaa/pb"
	"github.com/minio-io/cli"
	"github.com/minio-io/mc/pkg/console"
	"github.com/minio-io/minio/pkg/iodine"
	"github.com/minio-io/minio/pkg/utils/log"
)

func runCopyCmd(ctx *cli.Context) {
	if len(ctx.Args()) < 2 {
		cli.ShowCommandHelpAndExit(ctx, "cp", 1) // last argument is exit code
	}

	config, err := getMcConfig()
	if err != nil {
		log.Debug.Println(iodine.New(err, nil))
		console.Fatalf("mc: loading config file failed with following reason: [%s]\n", iodine.ToError(err))
	}

	// Convert arguments to URLs: expand alias, fix format...
	urls, err := getURLs(ctx.Args(), config.GetMapString("Aliases"))
	if err != nil {
		switch e := iodine.ToError(err).(type) {
		case errUnsupportedScheme:
			log.Debug.Println(iodine.New(err, nil))
			console.Fatalf("mc: parsing URL failed with following reason: [%s]\n", e)
		default:
			log.Debug.Println(iodine.New(err, nil))
			console.Fatalf("mc: parsing URL failed with following reason: [%s]\n", e)
		}
	}

	sourceURL := urls[0]   // First arg is source
	targetURLs := urls[1:] // 1 or more targets

	// perform copy
	if ctx.Bool("recursive") {
		errorMsg, err := doCopyCmdRecursive(mcClientManager{}, sourceURL, targetURLs)
		err = iodine.New(err, nil)
		if err != nil {
			if errorMsg == "" {
				errorMsg = "No error message present, please rerun with --debug and report a bug."
			}
			log.Debug.Println(err)
			console.Fatalf("mc: %s with following reason: [%s]\n", errorMsg, iodine.ToError(err))
		}
		return
	}
	errorMsg, err := doCopyCmd(mcClientManager{}, sourceURL, targetURLs)
	err = iodine.New(err, nil)
	if err != nil {
		if errorMsg == "" {
			errorMsg = "No error message present, please rerun with --debug and report a bug."
		}
		log.Debug.Println(err)
		console.Fatalf("mc: %s with following reason: [%s]\n", errorMsg, iodine.ToError(err))
	}
}

// doCopyCmd copies objects into and from a bucket or between buckets
func doCopyCmd(manager clientManager, sourceURL string, targetURLs []string) (string, error) {
	reader, length, hexMd5, err := manager.getSourceReader(sourceURL)
	if err != nil {
		msg := fmt.Sprintf("Reading from source URL: [%s] failed", sourceURL)
		return msg, iodine.New(err, nil)
	}
	defer reader.Close()

	writeClosers, err := getTargetWriters(manager, targetURLs, hexMd5, length)
	if err != nil {
		return "Writing to target URL failed", iodine.New(err, nil)
	}

	var writers []io.Writer
	for _, writer := range writeClosers {
		writers = append(writers, writer)
	}

	// set up progress bar
	var bar *pb.ProgressBar
	if !globalQuietFlag {
		bar = startBar(length)
		bar.Start()
		writers = append(writers, bar)
	}

	// write progress bar
	multiWriter := io.MultiWriter(writers...)

	// copy data to writers
	_, err = io.CopyN(multiWriter, reader, length)
	if err != nil {
		return "Copying data from source to target(s) failed", iodine.New(err, nil)
	}
	// close writers
	for _, writer := range writeClosers {
		err := writer.Close()
		if err != nil {
			err = iodine.New(err, nil)
		}
	}
	if err != nil {
		return "Connections still active, one or more writes may of failed.", iodine.New(err, nil)
	}
	if !globalQuietFlag {
		bar.Finish()
		console.Infoln("Success!")
	}
	return "", nil
}
