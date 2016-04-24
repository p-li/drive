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
	"time"
)

func multiplexOnChanMapResults(g *Commands, chanMap map[int]chan *keyValue) {
	spin := g.playabler()
	spin.play()
	defer spin.stop()

	for {
		if len(chanMap) < 1 {
			break
		}
		// Find the channel that has results
		for key, kvChan := range chanMap {
			select {
			case kv := <-kvChan:
				if kv == nil { // Sentinel emitted
					delete(chanMap, key)
					continue
				}
				if kv.value != nil {
					g.log.LogErrf("touch: %s %v\n", kv.key, kv.value.(error))
				}
			default:
			}
		}
	}

	return
}

func (g *Commands) Touch(byId bool) (err error) {
	// Arbitrary value for rate limiter
	throttle := time.Tick(1e9 / 10)

	chanMap := map[int]chan *keyValue{}

	for i, relToRootPath := range g.opts.Sources {
		fileId := ""
		if byId {
			fileId = relToRootPath
		}
		chanMap[i] = g.touch(relToRootPath, fileId, g.opts.Depth)
		<-throttle
	}

	multiplexOnChanMapResults(g, chanMap)
	return
}

func (g *Commands) TouchByMatch() (err error) {
	mq := matchQuery{
		dirPath: g.opts.Path,
		inTrash: false,
		titleSearches: []fuzzyStringsValuePair{
			{fuzzyLevel: Like, values: g.opts.Sources, inTrash: false},
		},
	}

	matches, err := g.rem.FindMatches(&mq)
	if err != nil {
		return err
	}

	throttle := time.Tick(1e9 / 10)
	chanMap := map[int]chan *keyValue{}

	i := 0
	for match := range matches {
		if match == nil {
			continue
		}

		chanMap[i] = g.touch(g.opts.Path+"/"+match.Name, match.Id, g.opts.Depth)
		<-throttle
		i += 1
	}

	multiplexOnChanMapResults(g, chanMap)
	return
}

func (g *Commands) touch(relToRootPath, fileId string, depth int) chan *keyValue {
	fileChan := make(chan *keyValue)

	go func() {
		kv := &keyValue{
			key:   relToRootPath,
			value: nil,
		}

		defer func() {
			fileChan <- kv
			close(fileChan)
		}()

		f, arg := g.rem.Touch, fileId
		if fileId == "" {
			f, arg = g.touchByPath, relToRootPath
		}
		file, err := f(arg)

		if err != nil {
			kv.value = err
			return
		}

		if g.opts.Verbose {
			g.log.Logf("%s: %v\n", relToRootPath, file.ModTime)
		}

		depth = decrementTraversalDepth(depth)
		if depth == 0 {
			return
		}

		if file.IsDir {
			childResults := make(chan chan *keyValue)
			// Arbitrary value for rate limiter
			throttle := time.Tick(1e9 * 2)
			childrenChan := g.rem.FindByParentId(file.Id, g.opts.Hidden)

			go func() {
				defer close(childResults)
				for child := range childrenChan {
					childResults <- g.touch(relToRootPath+"/"+child.Name, child.Id, depth)
					<-throttle
				}
			}()

			for childChan := range childResults {
				for childFile := range childChan {
					fileChan <- childFile
				}
			}
		}
	}()

	return fileChan
}

func (g *Commands) touchByPath(relToRootPath string) (*File, error) {
	file, err := g.rem.FindByPath(relToRootPath)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, ErrPathNotExists
	}
	return g.rem.Touch(file.Id)
}
