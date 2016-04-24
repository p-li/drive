// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package drive

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
)

var (
	ErrRcNoKeysMatched = errors.New("rcMapping: no keys matched")
)

const (
	DriveResourceConfiguration = ".driverc"
)

var (
	HomeEnvKey      = "HOME"
	HomeShellEnvKey = "$" + HomeEnvKey
	FsHomeDir       = os.Getenv(HomeEnvKey)
)

func kvifyCommentedFile(p, comment string) (kvMap map[string]string, err error) {
	var clauses []string
	kvMap = make(map[string]string)
	clauses, err = readCommentedFile(p, comment)
	if err != nil {
		return
	}

	for i, clause := range clauses {
		kvf, kvErr := splitAndStrip(clause, true)
		if kvErr != nil {
			err = kvErr
			return
		}

		value, ok := kvf.value.(string)
		if !ok {
			err = fmt.Errorf("clause %d: expected a string instead got %v", i, kvf.value)
			return
		}

		kvMap[kvf.key] = value
	}
	return
}

func ResourceMappings(rcPath string) (parsed map[string]interface{}, err error) {
	beginOpts := Options{Path: rcPath}
	rcPath, rcErr := beginOpts.rcPath()

	if rcErr != nil {
		err = rcErr
		return
	}

	rcMap, rErr := kvifyCommentedFile(rcPath, CommentStr)
	if rErr != nil {
		err = rErr
		return
	}
	return parseRCValues(rcMap)
}

func parseRCValues(rcMap map[string]string) (valueMappings map[string]interface{}, err error) {
	valueMappings = make(map[string]interface{})

	targetKeys := []struct {
		resolver resolverEmitter
		keys     []string
	}{
		{
			resolver: _boolfer, keys: []string{
				OcrKey, ConvertKey, CLIOptionFileBrowser, CLIOptionWebBrowser,
				CLIOptionVerboseKey, RecursiveKey, CLIOptionFiles, CLIOptionLongFmt,
				ForceKey, QuietKey, HiddenKey, NoPromptKey, NoClobberKey, IgnoreConflictKey,
				CLIOptionIgnoreNameClashes, CLIOptionIgnoreChecksum, CLIOptionFixClashesKey,
			},
		},
		{
			resolver: _intfer, keys: []string{
				PageSizeKey,
				DepthKey,
			},
		},
		{
			resolver: _stringfer, keys: []string{
				CLIOptionUnified, CLIOptionDiffBaseLocal,
				ExportsKey, ExcludeOpsKey, CLIOptionUnifiedShortKey,
				CLIOptionNotOwner, ExportsDirKey, CLIOptionExactTitle, AddressKey,
			},
		},
		{
			resolver: _stringArrayfer, keys: []string{
			// Add items that might need string array parsing and conversion here
			},
		},
	}

	accepted := make(map[string]typeResolver)
	for _, item := range targetKeys {
		resolver := item.resolver
		keys := item.keys
		for _, key := range keys {
			lowerKey := strings.ToLower(key)
			retr, ok := rcMap[lowerKey]
			if !ok {
				continue
			}

			tr := typeResolver{key: key, value: retr, resolver: resolver}
			accepted[lowerKey] = tr
		}
	}

	if false && len(accepted) < 1 {
		err = ErrRcNoKeysMatched
		return
	}

	for lowerKey, tResolver := range accepted {
		if value, err := tResolver.resolver(lowerKey, tResolver.value); err == nil {
			valueMappings[lowerKey] = value
		} else {
			fmt.Fprintf(os.Stderr, "rc: %s err %v\n", lowerKey, err)
		}
	}

	return
}

func rcMapToOptions(rcMap map[string]string) (*Options, error) {
	opts := &Options{}

	return opts, nil
}

func rcPath(absDirPath string) string {
	return path.Join(absDirPath, DriveResourceConfiguration)
}

func globalRcFile() string {
	ps := rcPath(FsHomeDir)
	return ps
}

func splitAndStrip(line string, resolveFromEnv bool) (kv keyValue, err error) {
	line = strings.Trim(line, " ")
	subStrCount := 2
	splits := strings.SplitN(line, "=", subStrCount)

	splitsLen := len(splits)
	if splitsLen < subStrCount-1 {
		return
	}

	if splitsLen < subStrCount {
		err = fmt.Errorf("expected <key>=<value> instead got %v", splits[:splitsLen])
		return
	}

	key, value := splits[0], splits[1:]
	joinedValue := strings.Join(NonEmptyTrimmedStrings(value...), " ")

	joinedValue = strings.Trim(joinedValue, " ")
	resolvedValue := joinedValue

	if resolveFromEnv {
		resolvedValue = os.ExpandEnv(resolvedValue)
	}

	if resolvedValue != "" && joinedValue != "" {
		joinedValue = resolvedValue
	}

	kv.key = key
	kv.value = joinedValue

	return
}

func JSONStringifySiftedCLITags(from interface{}, rcSourcePath string, defined map[string]bool) (repr string, err error) {
	rcMappings, rErr := ResourceMappings(rcSourcePath)

	if rErr != nil && !NotExist(rErr) {
		err = rErr
		return
	}

	cs := CliSifter{
		From:           from,
		Defaults:       rcMappings,
		AlreadyDefined: defined,
	}

	return SiftCliTags(&cs), err
}
