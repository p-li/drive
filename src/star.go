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

import "fmt"

func (g *Commands) Star(byId bool) error {
	return starring(g, true, byId)
}

func (g *Commands) UnStar(byId bool) error {
	return starring(g, false, byId)
}

func starring(g *Commands, starred, byId bool) (composedErr error) {
	kvChan := resolver(g, byId, g.opts.Sources, noopOnFile)

	verb := "Starred"
	if !starred {
		verb = "Unstarred"
	}

	for kv := range kvChan {
		file, ok := kv.value.(*File)
		if !ok {
			g.log.LogErrf("%s: %s\n", kv.key, kv.value)
			continue
		}

		if file == nil {
			continue
		}

		updatedFile, err := g.rem.updateStarred(file.Id, starred)
		if err != nil {
			composedErr = reComposeError(composedErr, fmt.Sprintf("%q %v", kv.key, err))
		} else if updatedFile != nil {
			name := fmt.Sprintf("%q", kv.key)
			if kv.key != updatedFile.Id {
				name = fmt.Sprintf("%s aka %q", name, updatedFile.Id)
			}
			g.log.LogErrf("%s %s\n", verb, name)
		}
	}

	return composedErr
}
