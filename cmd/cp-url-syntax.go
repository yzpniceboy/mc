/*
 * MinIO Client (C) 2015 MinIO, Inc.
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

package cmd

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/minio/cli"
	"github.com/minio/minio/pkg/console"
)

func checkCopySyntax(ctx context.Context, cliCtx *cli.Context, encKeyDB map[string][]prefixSSEPair, isMvCmd bool) {
	if len(cliCtx.Args()) < 2 {
		if isMvCmd {
			cli.ShowCommandHelpAndExit(cliCtx, "mv", 1) // last argument is exit code.
		}
		cli.ShowCommandHelpAndExit(cliCtx, "cp", 1) // last argument is exit code.
	}

	// extract URLs.
	URLs := cliCtx.Args()
	if len(URLs) < 2 {
		fatalIf(errDummy().Trace(cliCtx.Args()...), "Unable to parse source and target arguments.")
	}

	srcURLs := URLs[:len(URLs)-1]
	tgtURL := URLs[len(URLs)-1]
	isRecursive := cliCtx.Bool("recursive")
	rewind := cliCtx.String("rewind")
	timeRef := parseRewindFlag(rewind)

	// Verify if source(s) exists.
	for _, srcURL := range srcURLs {
		_, _, err := url2Stat(ctx, srcURL, "", false, encKeyDB, timeRef)
		if err != nil {
			console.Fatalf("Unable to validate source %s\n", srcURL)
		}
	}

	// Check if bucket name is passed for URL type arguments.
	url := newClientURL(tgtURL)
	if url.Host != "" {
		if url.Path == string(url.Separator) {
			fatalIf(errInvalidArgument().Trace(), fmt.Sprintf("Target `%s` does not contain bucket name.", tgtURL))
		}
	}

	if cliCtx.String(rdFlag) != "" && cliCtx.String(rmFlag) == "" {
		fatalIf(errInvalidArgument().Trace(), fmt.Sprintf("Both object retention flags `--%s` and `--%s` are required.\n", rdFlag, rmFlag))
	}

	if cliCtx.String(rdFlag) == "" && cliCtx.String(rmFlag) != "" {
		fatalIf(errInvalidArgument().Trace(), fmt.Sprintf("Both object retention flags `--%s` and `--%s` are required.\n", rdFlag, rmFlag))
	}

	operation := "copy"
	if isMvCmd {
		operation = "move"
	}

	// Guess CopyURLsType based on source and target URLs.
	copyURLsType, _, err := guessCopyURLType(ctx, srcURLs, tgtURL, isRecursive, encKeyDB, timeRef)
	if err != nil {
		fatalIf(errInvalidArgument().Trace(), "Unable to guess the type of "+operation+" operation.")
	}

	switch copyURLsType {
	case copyURLsTypeA: // File -> File.
		checkCopySyntaxTypeA(ctx, srcURLs, tgtURL, encKeyDB, isMvCmd, timeRef)
	case copyURLsTypeB: // File -> Folder.
		checkCopySyntaxTypeB(ctx, srcURLs, tgtURL, encKeyDB, isMvCmd, timeRef)
	case copyURLsTypeC: // Folder... -> Folder.
		checkCopySyntaxTypeC(ctx, srcURLs, tgtURL, isRecursive, encKeyDB, isMvCmd, timeRef)
	case copyURLsTypeD: // File1...FileN -> Folder.
		checkCopySyntaxTypeD(ctx, srcURLs, tgtURL, encKeyDB, isMvCmd, timeRef)
	default:
		fatalIf(errInvalidArgument().Trace(), "Unable to guess the type of "+operation+" operation.")
	}

	// Preserve functionality not supported for windows
	if cliCtx.Bool("preserve") && runtime.GOOS == "windows" {
		fatalIf(errInvalidArgument().Trace(), "Permissions are not preserved on windows platform.")
	}
}

// checkCopySyntaxTypeA verifies if the source and target are valid file arguments.
func checkCopySyntaxTypeA(ctx context.Context, srcURLs []string, tgtURL string, keys map[string][]prefixSSEPair, isMvCmd bool, timeRef time.Time) {
	// Check source.
	if len(srcURLs) != 1 {
		fatalIf(errInvalidArgument().Trace(), "Invalid number of source arguments.")
	}
	srcURL := srcURLs[0]
	_, srcContent, err := url2Stat(ctx, srcURL, "", false, keys, timeRef)
	fatalIf(err.Trace(srcURL), "Unable to stat source `"+srcURL+"`.")

	if !srcContent.Type.IsRegular() {
		fatalIf(errInvalidArgument().Trace(), "Source `"+srcURL+"` is not a file.")
	}
}

// checkCopySyntaxTypeB verifies if the source is a valid file and target is a valid folder.
func checkCopySyntaxTypeB(ctx context.Context, srcURLs []string, tgtURL string, keys map[string][]prefixSSEPair, isMvCmd bool, timeRef time.Time) {
	// Check source.
	if len(srcURLs) != 1 {
		fatalIf(errInvalidArgument().Trace(), "Invalid number of source arguments.")
	}
	srcURL := srcURLs[0]
	_, srcContent, err := url2Stat(ctx, srcURL, "", false, keys, timeRef)
	fatalIf(err.Trace(srcURL), "Unable to stat source `"+srcURL+"`.")

	if !srcContent.Type.IsRegular() {
		fatalIf(errInvalidArgument().Trace(srcURL), "Source `"+srcURL+"` is not a file.")
	}

	// Check target.
	if _, tgtContent, err := url2Stat(ctx, tgtURL, "", false, keys, timeRef); err == nil {
		if !tgtContent.Type.IsDir() {
			fatalIf(errInvalidArgument().Trace(tgtURL), "Target `"+tgtURL+"` is not a folder.")
		}
	}
}

// checkCopySyntaxTypeC verifies if the source is a valid recursive dir and target is a valid folder.
func checkCopySyntaxTypeC(ctx context.Context, srcURLs []string, tgtURL string, isRecursive bool, keys map[string][]prefixSSEPair, isMvCmd bool, timeRef time.Time) {
	// Check source.
	if len(srcURLs) != 1 {
		fatalIf(errInvalidArgument().Trace(), "Invalid number of source arguments.")
	}

	// Check target.
	if _, tgtContent, err := url2Stat(ctx, tgtURL, "", false, keys, timeRef); err == nil {
		if !tgtContent.Type.IsDir() {
			fatalIf(errInvalidArgument().Trace(tgtURL), "Target `"+tgtURL+"` is not a folder.")
		}
	}

	for _, srcURL := range srcURLs {
		c, srcContent, err := url2Stat(ctx, srcURL, "", false, keys, timeRef)
		// incomplete uploads are not necessary for copy operation, no need to verify for them.
		isIncomplete := false
		if err != nil {
			if !isURLPrefixExists(srcURL, isIncomplete) {
				fatalIf(err.Trace(srcURL), "Unable to stat source `"+srcURL+"`.")
			}
			// No more check here, continue to the next source url
			continue
		}

		if srcContent.Type.IsDir() {
			// Require --recursive flag if we are copying a directory
			if !isRecursive {
				operation := "copy"
				if isMvCmd {
					operation = "move"
				}
				fatalIf(errInvalidArgument().Trace(srcURL), fmt.Sprintf("To %v a folder requires --recursive flag.", operation))
			}

			// Check if we are going to copy a directory into itself
			if isURLContains(srcURL, tgtURL, string(c.GetURL().Separator)) {
				operation := "Copying"
				if isMvCmd {
					operation = "Moving"
				}
				fatalIf(errInvalidArgument().Trace(), fmt.Sprintf("%v a folder into itself is not allowed.", operation))
			}
		}
	}

}

// checkCopySyntaxTypeD verifies if the source is a valid list of files and target is a valid folder.
func checkCopySyntaxTypeD(ctx context.Context, srcURLs []string, tgtURL string, keys map[string][]prefixSSEPair, isMvCmd bool, timeRef time.Time) {
	// Source can be anything: file, dir, dir...
	// Check target if it is a dir
	if _, tgtContent, err := url2Stat(ctx, tgtURL, "", false, keys, timeRef); err == nil {
		if !tgtContent.Type.IsDir() {
			fatalIf(errInvalidArgument().Trace(tgtURL), "Target `"+tgtURL+"` is not a folder.")
		}
	}
}
