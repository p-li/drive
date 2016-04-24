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
	"fmt"
	"strconv"
	"strings"
)

const (
	TBool = iota
	TRune
	TInt
	TInt64
	TUInt
	TString
	TStringArray
	TUInt64
)

type resolverEmitter func(varname, v string) (interface{}, error)

type typeEmitterPair struct {
	value        string
	valueEmitter func(v string) interface{}
}

type typeResolver struct {
	key      string
	value    string
	resolver resolverEmitter
}

type tParsed int

func (t tParsed) String() string {
	switch t {
	case TBool:
		return "bool"
	case TInt:
		return "int"
	case TInt64:
		return "int64"
	case TRune:
		return "rune"
	case TString:
		return "string"
	case TStringArray:
		return "[]string"
	case TUInt64:
		return "uint64"
	default:
		return "unknown"
	}
}

func parseErrorer(varname string, t tParsed, value interface{}, args ...interface{}) error {
	return fmt.Errorf("%s: got \"%v\", expected type %s %v", varname, value, t, args)
}

func _boolfer(varname, strValue string) (interface{}, error) {
	retr, rErr := strconv.ParseBool(strValue)
	var value bool
	var err error

	if rErr == nil {
		value = retr
	} else {
		err = parseErrorer(varname, TBool, value, rErr)
	}

	return value, err
}

func _stringfer(varname, value string) (interface{}, error) {
	// TODO: perform strips and trims
	return value, nil
}

func _stringArrayfer(varname, value string) (interface{}, error) {
	splits := NonEmptyTrimmedStrings(strings.Split(value, ",")...)
	return splits, nil
}

func stringArrayfer(varname, value string, optSave *Options) error {
	var splits []string
	splitsInterface, err := _stringArrayfer(varname, value)
	if err != nil {
		return err
	}

	splits, ok := splitsInterface.([]string)
	if !ok {
		return fmt.Errorf("varname: %s value: %s. Failed to create []string", varname, value)
	}

	var ptr *[]string

	varnameLower := strings.ToLower(varname)

	switch varnameLower {
	case ExportsKey:
		ptr = &optSave.Exports

	case ExcludeOpsKey:
		optSave.ExcludeCrudMask = CrudAtoi(splits...)
		return nil
	}

	if ptr != nil {
		*ptr = splits
	}
	return nil
}

func _intfer(varname, strValue string) (interface{}, error) {
	var value int
	var err error

	v64, vErr := strconv.ParseInt(strValue, 10, 32)
	if vErr == nil {
		value = int(v64)
	} else {
		err = parseErrorer(varname, TInt, value, vErr)
	}

	return value, err
}
