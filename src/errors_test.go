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

import (
	"fmt"
	"strings"
	"testing"
)

func TestErrors(t *testing.T) {
	testCases := [...]struct {
		e             *Error
		wantErrString string
		wantCode      int
	}{
		0: {
			e: &Error{
				status: "foo",
				code:   StatusIllogicalState,
			},
			wantErrString: "foo",
			wantCode:      int(StatusIllogicalState),
		},
		1: {
			e: &Error{
				err:    fmt.Errorf("bar"),
				status: "first",
			},
			wantErrString: "first bar",
			wantCode:      0,
		},
		2: {
			e: &Error{
				err:    fmt.Errorf("bar"),
				status: "first",
				code:   StatusInvalidGoogleAPIQuery,
			},
			wantErrString: "first bar",
			wantCode:      int(StatusInvalidGoogleAPIQuery),
		},
		3: {
			e:             makeError(fmt.Errorf("golang"), StatusNonExistantRemote),
			wantErrString: "golang",
			wantCode:      int(StatusNonExistantRemote),
		},
		4: {
			e:             makeErrorWithStatus("drive over errthang", fmt.Errorf("truu"), StatusUnresolvedConflicts),
			wantErrString: "drive over errthang truu",
			wantCode:      int(StatusUnresolvedConflicts),
		},
	}

	for i, tc := range testCases {
		gotString, wantString := tc.e.Error(), tc.wantErrString
		if !strings.EqualFold(gotString, wantString) {
			t.Errorf("#%d got=%q want=%q", i, gotString, wantString)
		}

		gotCode, wantCode := tc.e.Code(), tc.wantCode
		if gotCode != wantCode {
			t.Errorf("#%d code: got=%v want=%v", i, gotCode, wantCode)
		}
	}
}
