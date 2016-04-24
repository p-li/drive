// Copyright 2016 Google Inc. All Rights Reserved.
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

type ErrorStatus int

const (
	StatusGeneric                     ErrorStatus = 1
	StatusAuthenticationFailed        ErrorStatus = 2
	StatusRetriesExhausted            ErrorStatus = 3
	StatusDownloadFailed              ErrorStatus = 4
	StatusPullFailed                  ErrorStatus = 5
	StatusGoogleDocNonExportAttempted ErrorStatus = 6
	StatusInvalidGoogleAPIQuery       ErrorStatus = 7
	StatusIllogicalState              ErrorStatus = 8
	StatusNonExistantRemote           ErrorStatus = 9
	StatusRemoteLookupFailed          ErrorStatus = 10
	StatusLocalLookupFailed           ErrorStatus = 11
	StatusNetLookupFailed             ErrorStatus = 12
	StatusClashesDetected             ErrorStatus = 13
	StatusClashFixingAborted          ErrorStatus = 14
	StatusMkdirFailed                 ErrorStatus = 15
	StatusNoMatchesFound              ErrorStatus = 16
	StatusUnresolvedConflicts         ErrorStatus = 17
	StatusCannotPrompt                ErrorStatus = 18
	StatusOverwriteAttempted          ErrorStatus = 19
	StatusImmutableOperationAttempted ErrorStatus = 20
	StatusInvalidArguments            ErrorStatus = 21
	StatusNamedPipeReadAttempt        ErrorStatus = 22
	StatusContentTooLarge             ErrorStatus = 23
)

type Error struct {
	code   ErrorStatus
	status string
	err    error
}

func (e Error) Error() string {
	joins := []string{}
	if e.status != "" {
		joins = append(joins, e.status)
	}
	if e.err != nil {
		joins = append(joins, e.err.Error())
	}
	return sepJoin(" ", joins...)
}

func (e Error) Code() int {
	return int(e.code)
}

func makeError(err error, code ErrorStatus) *Error {
	return &Error{
		code: code,
		err:  err,
	}
}

func makeErrorWithStatus(status string, err error, code ErrorStatus) *Error {
	e := makeError(err, code)
	e.status = status
	return e
}

func googleDocNonExportErr(err error) *Error {
	return makeError(err, StatusGoogleDocNonExportAttempted)
}

func downloadFailedErr(err error) *Error {
	return makeError(err, StatusDownloadFailed)
}

func illogicalStateErr(err error) *Error {
	return makeError(err, StatusIllogicalState)
}

func invalidGoogleAPIQueryErr(err error) *Error {
	return makeError(err, StatusInvalidGoogleAPIQuery)
}

func nonExistantRemoteErr(err error) *Error {
	return makeError(err, StatusNonExistantRemote)
}

func netLookupFailedErr(err error) *Error {
	return makeError(err, StatusNetLookupFailed)
}

func clashesDetectedErr(err error) *Error {
	return makeError(err, StatusClashesDetected)
}

func clashFixingAbortedErr(err error) *Error {
	return makeError(err, StatusClashFixingAborted)
}

func mkdirFailedErr(err error) *Error {
	return makeError(err, StatusMkdirFailed)
}

func noMatchesFoundErr(err error) *Error {
	return makeError(err, StatusNoMatchesFound)
}

func unresolvedConflictsErr(err error) *Error {
	return makeError(err, StatusUnresolvedConflicts)
}

func pullFailedErr(err error) *Error {
	return makeError(err, StatusPullFailed)
}

func cannotPromptErr(err error) *Error {
	return makeError(err, StatusCannotPrompt)
}

func overwriteAttemptedErr(err error) *Error {
	return makeError(err, StatusOverwriteAttempted)
}

func immutableAttemptErr(err error) *Error {
	return makeError(err, StatusImmutableOperationAttempted)
}

func remoteLookupErr(err error) *Error {
	return makeError(err, StatusRemoteLookupFailed)
}

func invalidArgumentsErr(err error) *Error {
	return makeError(err, StatusInvalidArguments)
}

func namedPipeReadAttemptErr(err error) *Error {
	return makeError(err, StatusNamedPipeReadAttempt)
}

func contentTooLargeErr(err error) *Error {
	return makeError(err, StatusContentTooLarge)
}
