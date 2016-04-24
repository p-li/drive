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
)

const (
	fileIdWidth = -48
)

func idPrintAndRecurse(g *Commands, parentId, relToRootPath string, depth int) (err error) {
	if depth == 0 {
		return
	}

	// Paths vary greatly in length but fileIds don't vary that much
	g.log.Logf("%*s %s\n", int(fileIdWidth), customQuote(parentId), customQuote(relToRootPath))

	decrementedDepth := decrementTraversalDepth(depth)
	if decrementedDepth == 0 { // No need to recurse if depth is already 0
		return
	}

	children := g.rem.FindByParentId(parentId, g.opts.Hidden)

	separatorPrefix := relToRootPath
	if rootLike(separatorPrefix) {
		// Avoid a situation where you have Join("/", "/", "a") -> "//a"
		separatorPrefix = ""
	}

	for child := range children {
		if child == nil {
			continue
		}
		childRelToRootPath := sepJoin(RemoteSeparator, separatorPrefix, child.Name)
		cErr := idPrintAndRecurse(g, child.Id, childRelToRootPath, decrementedDepth)
		if cErr != nil {
			err = combineErrors(err, cErr)
		}
	}

	return err
}

func (g *Commands) Id() (err error) {
	header := fmt.Sprintf("%*s %s", int(fileIdWidth), "FileId", "Relative Path")
	headerPrinted := false

	for _, relToRootPath := range g.opts.Sources {
		remotes := g.rem.FindByPathM(relToRootPath)
		iterCount := uint64(0)
		for rem := range remotes {
			if rem == nil {
				err = reComposeError(err, fmt.Sprintf("%s does not exist remotely", customQuote(relToRootPath)))
				continue
			}

			if !headerPrinted {
				g.log.Logln(header)
				headerPrinted = true
			}

			iterCount++
			cErr := idPrintAndRecurse(g, rem.Id, relToRootPath, g.opts.Depth)
			if cErr != nil {
				err = combineErrors(err, cErr)
			}
		}

		if iterCount < 1 {
			err = reComposeError(err, fmt.Sprintf("%s not matched", customQuote(relToRootPath)))
		}
	}

	return err
}
