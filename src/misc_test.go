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
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
)

func callerFilepath() string {
	_, p, _, _ := runtime.Caller(1)
	return p
}

func TestRemoteOpToChangerTranslator(t *testing.T) {
	g := &Commands{}
	now := time.Now()

	cases := []struct {
		change   *Change
		name     string
		wantedFn func(*Change) error
	}{
		{change: &Change{Src: nil, Dest: nil}, wantedFn: nil, name: "nil"},
		{change: &Change{Src: &File{}, Dest: &File{}}, wantedFn: nil, name: "nil"},
		{change: &Change{Src: &File{}, Dest: nil}, wantedFn: g.remoteAdd, name: "remoteAdd"},
		{change: &Change{Src: nil, Dest: &File{}}, wantedFn: g.remoteTrash, name: "remoteTrash"},
		{
			change: &Change{
				Dest: &File{ModTime: now},
				Src:  &File{ModTime: now.Add(time.Hour)},
			},
			wantedFn: g.remoteMod, name: "remoteMod",
		},
		{
			change: &Change{
				Dest: &File{ModTime: now},
				Src:  &File{ModTime: now},
			},
			wantedFn: nil, name: "noop",
		},
	}

	for _, tc := range cases {
		got := remoteOpToChangerTranslator(g, tc.change)
		vptr1 := reflect.ValueOf(got).Pointer()
		vptr2 := reflect.ValueOf(tc.wantedFn).Pointer()

		if vptr1 != vptr2 {
			t.Errorf("expected %q expected (%v) got (%v)", tc.name, tc.wantedFn, got)
		}
	}
}

func TestLocalOpToChangerTranslator(t *testing.T) {
	g := &Commands{}
	now := time.Now()

	cases := []struct {
		change   *Change
		name     string
		wantedFn func(*Change, []string) error
	}{
		{change: &Change{Src: nil, Dest: nil}, wantedFn: nil, name: "nil"},
		{change: &Change{Src: &File{}, Dest: &File{}}, wantedFn: nil, name: "nil"},
		{
			change:   &Change{Src: &File{}, Dest: nil},
			wantedFn: g.localAdd, name: "localAdd",
		},
		{
			change:   &Change{Dest: nil, Src: &File{}},
			wantedFn: g.localAdd, name: "localAdd",
		},
		{
			change:   &Change{Src: nil, Dest: &File{}},
			wantedFn: g.localDelete, name: "localDelete",
		},
		{
			change: &Change{
				Src:  &File{ModTime: now},
				Dest: &File{ModTime: now.Add(time.Hour)},
			},
			wantedFn: g.localMod, name: "localMod",
		},
		{
			change: &Change{
				Dest: &File{ModTime: now},
				Src:  &File{ModTime: now},
			},
			wantedFn: nil, name: "noop",
		},
	}

	for _, tc := range cases {
		got := localOpToChangerTranslator(g, tc.change)
		vptr1 := reflect.ValueOf(got).Pointer()
		vptr2 := reflect.ValueOf(tc.wantedFn).Pointer()

		if vptr1 != vptr2 {
			t.Errorf("expected %q expected (%v) got (%v)", tc.name, tc.wantedFn, got)
		}
	}
}

func TestRetryableErrorCheck(t *testing.T) {
	cases := []struct {
		value              interface{}
		success, retryable bool
		comment            string
	}{
		{
			value: nil, success: false, retryable: true,
			comment: "a nil tuple is retryable but not successful",
		},
		{
			value: t, success: false, retryable: true,
			comment: "t value is not a tuple, is retryable but not successful",
		},
		{
			value: &tuple{first: nil, last: nil}, success: true, retryable: false,
			comment: "last=nil representing a nil error so success, unretryable",
		},
		{
			value:   &tuple{first: nil, last: fmt.Errorf("flux")},
			success: false, retryable: true,
			comment: "last!=nil, non-familiar error so unsuccessful, retryable",
		},
		{
			value: &tuple{
				first: "",
				last: &googleapi.Error{
					Message: "This is an error",
				},
			},
			success: false, retryable: false,
			comment: "last!=nil, familiar error so unsuccessful, retryable:: statusCode undefined",
		},
		{
			value: &tuple{
				first: "",
				last: &googleapi.Error{
					Code:    500,
					Message: "This is an error",
				},
			},
			success: false, retryable: true,
			comment: "last!=nil, familiar error so unsuccessful, retryable:: statusCode 500",
		},
		{
			value: &tuple{
				first: nil,
				last: &googleapi.Error{
					Code:    401,
					Message: "401 right here",
				},
			},
			success: false, retryable: true,
			comment: "last!=nil, 401 must be retryable",
		},
		{
			value: &tuple{
				first: nil,
				last: &googleapi.Error{
					Code:    409,
					Message: "409 right here",
				},
			},
			success: false, retryable: false,
			comment: "last!=nil, 409 is unclassified so unretryable",
		},
		{
			value: &tuple{
				first: nil,
				last: &googleapi.Error{
					Code:    403,
					Message: "403 right here",
				},
			},
			success: false, retryable: true,
			comment: "last!=nil, 403 is retryable",
		},
		{
			value: &tuple{
				first: nil,
				last: &googleapi.Error{
					Code:    500,
					Message: MsgErrFileNotMutable,
				},
			},
			success: false, retryable: false,
			comment: "issue #472 FileNotMutable is unretryable",
		},
		{
			value: &tuple{
				first: nil,
				last: &googleapi.Error{
					Code:    500,
					Message: strings.ToLower(MsgErrFileNotMutable),
				},
			},
			success: false, retryable: false,
			comment: "issue #472 FileNotMutable is unretryable, casefold held",
		},
		{
			value: &tuple{
				first: nil,
				last: &googleapi.Error{
					Code:    501,
					Message: strings.ToUpper(MsgErrFileNotMutable),
				},
			},
			success: false, retryable: false,
			comment: "issue #472 FileNotMutable is unretryable, casefold held",
		},
	}

	for _, tc := range cases {
		success, retryable := retryableErrorCheck(tc.value)
		if success != tc.success {
			t.Errorf("%v success got %v expected %v", tc.value, success, tc.success)
		}
		if retryable != tc.retryable {
			t.Errorf("%v retryable got %v expected %v: %q", tc.value, retryable, tc.retryable, tc.comment)
		}
	}
}

func TestDriveIgnore(t *testing.T) {
	testCases := []struct {
		clauses          []string
		mustErr          bool
		nilIgnorer       bool
		excludesExpected []string
		includesExpected []string
		comment          string
		mustBeIgnored    []string
		mustNotBeIgnored []string
	}{
		{clauses: []string{}, nilIgnorer: true, comment: "no clauses in"},
		{
			clauses: []string{"#this is a comment"}, nilIgnorer: false,
			comment: "plain commented file",
		},
		{
			comment:          "intentionally unescaped '.'",
			clauses:          []string{".git", ".docx$"},
			mustBeIgnored:    []string{"bgits", "frogdocx"},
			mustNotBeIgnored: []string{"", "  ", "frogdocxs"},
		},
		{
			comment:          "entirely commented, so all clauses should be skipped",
			clauses:          []string{"^#"},
			mustBeIgnored:    []string{"#patch", "#   ", "#", "#Like this one", "#\\.git"},
			mustNotBeIgnored: []string{"", "  ", "src/misc_test.go"},
		},
		{
			comment:       "strictly escaped '.'",
			clauses:       []string{"\\.git", "\\.docx$"},
			mustBeIgnored: []string{".git", "drive.docx", ".docx"},
			mustNotBeIgnored: []string{
				"", "  ", "frogdocxs", "digit", "drive.docxs",
				"drive.docxx", "drive.", ".drive", ".docx ",
			},
		},
		{
			comment:       "strictly escaped '.'",
			clauses:       []string{"^\\.", "#!\\.driveignore"},
			mustBeIgnored: []string{".git", ".driveignore", ".bashrc"},
			mustNotBeIgnored: []string{
				"", "  ", "frogdocxs", "digit", "drive.docxs",
				"drive.docxx", "drive.", " .drive", "a.docx ",
			},
		},
		{
			comment:       "include vs exclude issue #535",
			clauses:       []string{"\\.", "!^\\.docx$", "!\\.bashrc", "#!\\.driveignore"},
			mustBeIgnored: []string{".git", "drive.docx", ".docx ", ".driveignore"},
			mustNotBeIgnored: []string{
				".docx", ".bashrc",
			},
		},
	}

	for _, tc := range testCases {
		ignorer, err := ignorerByClause(tc.clauses...)
		if tc.mustErr {
			if err == nil {
				t.Fatalf("expected to err with clause %v comment %q", tc.clauses, tc.comment)
			}
		} else if err != nil {
			t.Fatalf("%v should not err. Got %v", tc.clauses, err)
		}

		if tc.nilIgnorer {
			if ignorer != nil {
				t.Fatalf("ignorer for (%v)(%q) expected to be nil, got %p", tc.clauses, tc.comment, ignorer)
			}
		} else if ignorer == nil {
			t.Fatalf("ignorer not expected to be nil for (%v) %q", tc.clauses, tc.comment)
		}

		if !tc.nilIgnorer && ignorer != nil {
			for _, expectedPass := range tc.mustBeIgnored {
				if !ignorer(expectedPass) {
					t.Errorf("%q: %q must be ignored", tc.comment, expectedPass)
				}
			}

			for _, expectedFail := range tc.mustNotBeIgnored {
				if ignorer(expectedFail) {
					t.Errorf("%q: %q must not be ignored", tc.comment, expectedFail)
				}
			}
		}
	}
}

func TestReadFile(t *testing.T) {
	ownFilepath := callerFilepath()
	comment := `
// A comment right here intentionally put that will self read and consumed.
+  A follow up right here and now.
`
	clauses, err := readCommentedFile(ownFilepath, "//")
	if err != nil {
		t.Fatalf("%q is currently being run and should be read successfully, instead got err %v", ownFilepath, err)
	}

	if len(clauses) < 1 {
		t.Errorf("expecting at least one line in this file %q", ownFilepath)
	}

	restitched := strings.Join(clauses, "\n")
	if strings.Index(restitched, comment) != -1 {
		t.Errorf("%q should have been ignored as a comment", comment)
	}
}

func TestCustomQuote(t *testing.T) {
	// https://github.com/golang/go/issues/11511
	// https://github.com/odeke-em/drive/issues/250

	testCases := []struct {
		sample, want string
	}{
		{
			sample: "", want: "\"\"",
		},
		{
			sample: "全角スペース　含みます/", want: "\"全角スペース　含みます/\"",
		},
		{
			sample: "this is a test", want: "\"this is a test\"",
		},
		{
			sample: "this ' is a test", want: "\"this ' is a test\"",
		},
		{
			sample: "全角スペース\"　含みます/", want: "\"全角スペース\\\"　含みます/\"",
		},
		{
			sample: "久聞大名  久聞大名", want: "\"久聞大名  久聞大名\"",
		},
		{
			sample: "Go: для начинающих и профессионалов (18:30)", want: "\"Go: для начинающих и профессионалов (18:30)\"",
		},
		{
			sample: "Go:\\ для начинающих и профессионалов (18:30)", want: "\"Go:\\\\ для начинающих и профессионалов (18:30)\"",
		},
	}

	for _, tc := range testCases {
		got := customQuote(tc.sample)
		if got != tc.want {
			t.Errorf("given sample: %v, wanted %v, got %v", tc.sample, tc.want, got)
		}
	}
}

func TestHttpOk(t *testing.T) {
	testCases := []struct {
		sample int
		want   bool
	}{
		{sample: 200, want: true},
		{sample: 201, want: true},
		{sample: 210, want: true},
		{sample: 290, want: true},
		{sample: 299, want: true},
		{sample: -200, want: false},
		{sample: -200000000000, want: false},
		{sample: 300, want: false},
		{sample: 403, want: false},
		{sample: 500, want: false},
		{sample: 100, want: false},
		{sample: 0, want: false},
		{sample: -1, want: false},
	}

	for _, tc := range testCases {
		if got, want := httpOk(tc.sample), tc.want; got != want {
			t.Errorf("given sample %v, expected %v instead got %v", tc.sample, want, got)
		}
	}
}

func TestCrudToAtoi(t *testing.T) {
	testCases := []struct {
		specimen []string
		want     CrudValue
		comment  string
	}{
		{
			specimen: []string{"create", "delete", "update", "read"},
			want:     Create | Delete | Update | Read,
		},
		{
			specimen: []string{"c", "r", "u", "d`"},
			want:     Create | Read | Update | Delete,
			comment:  "short forms being used",
		},
		{
			specimen: []string{"trim", "create", "update", "bank`"},
			want:     Create | Update,
		},
		{
			specimen: []string{"interfere", "extrict", "influence", "bank`"},
			want:     None,
		},
		{
			specimen: []string{"", "", "", "ReAd"},
			want:     Read,
		},
		{
			specimen: []string{"", "", "", ""},
			want:     None,
		},
	}

	for _, tc := range testCases {
		if got, want := CrudAtoi(tc.specimen...), tc.want; got != want {
			t.Errorf("given specimen %v, expected %q instead got %q", tc.specimen, want, got)
		}
	}
}
