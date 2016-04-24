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
	"path/filepath"
	"strings"
)

func (g *Commands) ListClashes(byId bool) error {
	spin := g.playabler()
	spin.play()
	clashes, err := listClashes(g, g.opts.Sources, byId)
	spin.stop()

	if len(clashes) < 1 {
		if err == nil {
			return fmt.Errorf("no clashes exist!")
		} else {
			return err
		}
	}

	if err != nil && err != ErrClashesDetected {
		return err
	}

	warnClashesPersist(g.log, clashes)
	return nil
}

func (g *Commands) FixClashes(byId bool) error {
	spin := g.playabler()
	spin.play()
	clashes, err := listClashes(g, g.opts.Sources, byId)
	spin.stop()

	if len(clashes) < 1 {
		return err
	}

	if err != nil && err != ErrClashesDetected {
		return err
	}

	return autoRenameClashes(g, clashes)
}

func findClashesForChildren(g *Commands, parentId, relToRootPath string, depth int) (clashes []*Change, err error) {
	if depth == 0 {
		return
	}

	memoized := map[string][]*File{}
	children := g.rem.FindByParentId(parentId, g.opts.Hidden)
	decrementedDepth := decrementTraversalDepth(depth)

	discoveryOrder := []string{}
	for child := range children {
		if child == nil {
			continue
		}
		cluster, alreadyDiscovered := memoized[child.Name]
		if !alreadyDiscovered {
			discoveryOrder = append(discoveryOrder, child.Name)
		}

		cluster = append(cluster, child)
		memoized[child.Name] = cluster
	}

	separatorPrefix := relToRootPath
	if rootLike(separatorPrefix) {
		// Avoid a situation where you have Join("/", "/", "a") -> "//a"
		separatorPrefix = ""
	}

	// To preserve the discovery order
	for _, commonKey := range discoveryOrder {
		cluster, _ := memoized[commonKey]
		fullRelToRootPath := sepJoin(RemoteSeparator, separatorPrefix, commonKey)
		nameClashesPresent := len(cluster) > 1

		for _, rem := range cluster {
			if nameClashesPresent {
				change := &Change{
					Src: rem, g: g,
					Path:   fullRelToRootPath,
					Parent: g.opts.Path,
				}

				clashes = append(clashes, change)
			}

			ccl, cErr := findClashesForChildren(g, rem.Id, fullRelToRootPath, decrementedDepth)
			clashes = append(clashes, ccl...)
			if cErr != nil {
				err = combineErrors(err, cErr)
			}
		}
	}

	return
}

func findClashesByPath(g *Commands, relToRootPath string, depth int) (clashes []*Change, err error) {
	if depth == 0 {
		return
	}

	remotes := g.rem.FindByPathM(relToRootPath)

	iterCount := uint64(0)
	clashThresholdCount := uint64(1)

	cl := []*Change{}

	for rem := range remotes {
		if rem == nil {
			continue
		}

		iterCount++

		change := &Change{
			Src: rem, g: g,
			Path:   relToRootPath,
			Parent: g.opts.Path,
		}

		cl = append(cl, change)
	}

	if iterCount > clashThresholdCount {
		clashes = append(clashes, cl...)
	}

	decrementedDepth := decrementTraversalDepth(depth)
	if decrementedDepth == 0 {
		return
	}

	for _, change := range cl {
		ccl, cErr := findClashesForChildren(g, change.Src.Id, change.Path, decrementedDepth)
		if cErr != nil {
			err = combineErrors(err, cErr)
		}

		clashes = append(clashes, ccl...)
	}

	return
}

func listClashes(g *Commands, sources []string, byId bool) (clashes []*Change, err error) {
	for _, relToRootPath := range g.opts.Sources {
		cclashes, cErr := findClashesByPath(g, relToRootPath, g.opts.Depth)

		clashes = append(clashes, cclashes...)

		if cErr != nil {
			err = combineErrors(err, cErr)
		}
	}

	return
}

func autoRenameClashes(g *Commands, clashes []*Change) error {
	clashesMap := map[string][]*Change{}

	for _, clash := range clashes {
		group := clashesMap[clash.Path]
		if clash.Src == nil {
			continue
		}
		clashesMap[clash.Path] = append(group, clash)
	}

	renames := make([]renameOp, 0, len(clashesMap)) // we will have at least len(clashesMap) renames

	for commonPath, group := range clashesMap {
		ext := filepath.Ext(commonPath)
		name := strings.TrimSuffix(commonPath, ext)
		nextIndex := 0

		for i, n := 1, len(group); i < n; i++ { // we can leave first item with original name
			var newName string
			for {
				newName = fmt.Sprintf("%v_%d%v", name, nextIndex, ext)
				nextIndex++

				dupCheck, err := g.rem.FindByPath(newName)
				if err != nil && err != ErrPathNotExists {
					return err
				}

				if dupCheck == nil {
					newName = filepath.Base(newName)
					break
				}
			}

			r := renameOp{newName: newName, change: group[i], originalPath: commonPath}
			renames = append(renames, r)
		}
	}

	if g.opts.canPrompt() {
		g.log.Logln("Some clashes found, we can fix them by following renames:")
		for _, r := range renames {
			g.log.Logf("%v %v -> %v\n", r.originalPath, r.change.Src.Id, r.newName)
		}
		status := promptForChanges("Proceed with the changes [Y/N] ? ")
		if !accepted(status) {
			return ErrClashFixingAborted
		}
	}

	var composedError error

	quot := customQuote
	for _, r := range renames {
		message := fmt.Sprintf("Renaming %s %s -> %s\n", quot(r.originalPath), quot(r.change.Src.Id), quot(r.newName))
		_, err := g.rem.rename(r.change.Src.Id, r.newName)
		if err == nil {
			g.log.Log(message)
			continue
		}

		composedError = reComposeError(composedError, fmt.Sprintf("%s err: %v", message, err))
	}

	return composedError
}
