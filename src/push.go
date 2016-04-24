// Copyright 2013 Google Inc. All Rights Reserved.
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
	"os"
	"os/signal"
	gopath "path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/odeke-em/drive/config"
	"github.com/odeke-em/semalim"
)

var mkdirAllMu = sync.Mutex{}

// Pushes to remote if local path exists and in a gd context. If path is a
// directory, it recursively pushes to the remote if there are local changes.
// It doesn't check if there are local changes if isForce is set.
func (g *Commands) Push() (err error) {
	defer g.clearMountPoints()

	var cl []*Change

	g.log.Logln("Resolving...")

	spin := g.playabler()
	spin.play()

	// To Ensure mount points are cleared in the event of external exceptions
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		_ = <-c
		spin.stop()
		g.clearMountPoints()
		os.Exit(1)
	}()

	clashes := []*Change{}

	rootAbsPath := g.context.AbsPathOf("")
	destAbsPath := g.context.AbsPathOf(g.opts.Destination)
	remoteDestRelPath, err := filepath.Rel(rootAbsPath, destAbsPath)
	if err != nil {
		return err
	}
	for _, relToRootPath := range g.opts.Sources {
		fsAbsPath := g.context.AbsPathOf(relToRootPath)
		// Join this relative path to that of the remote relative path of the destination.
		relToDestPath := remotePathJoin(remoteDestRelPath, relToRootPath)
		ccl, cclashes, cErr := g.changeListResolve(relToDestPath, fsAbsPath, true)

		clashes = append(clashes, cclashes...)
		if cErr != nil && cErr != ErrClashesDetected {
			spin.stop()
			return cErr
		}
		if len(ccl) > 0 {
			cl = append(cl, ccl...)
		}
	}

	if len(clashes) >= 1 {
		if g.opts.FixClashes {
			err := autoRenameClashes(g, clashes)
			if err == nil {
				g.log.Logln(MsgClashesFixedNowRetry)
			}
			return err
		}

		warnClashesPersist(g.log, clashes)
		return ErrClashesDetected
	}

	mount := g.opts.Mount
	if mount != nil {
		for _, mt := range mount.Points {
			ccl, _, cerr := lonePush(g, rootAbsPath, mt.Name, mt.MountPath)
			if cerr == nil {
				cl = append(cl, ccl...)
			}
		}
	}

	spin.stop()

	nonConflictsPtr, conflictsPtr := g.resolveConflicts(cl, true)
	if conflictsPtr != nil {
		warnConflictsPersist(g.log, *conflictsPtr)
		return unresolvedConflictsErr(fmt.Errorf("conflicts have prevented a push operation"))
	}

	nonConflicts := *nonConflictsPtr

	pushSize, modSize := reduceToSize(cl, SelectDest|SelectSrc)

	// Compensate for deletions and modifications
	pushSize -= modSize

	// Warn about (near) quota exhaustion
	quotaStatus, quotaErr := g.QuotaStatus(pushSize)
	if quotaErr != nil {
		return quotaErr
	}

	unSafe := false
	switch quotaStatus {
	case AlmostExceeded:
		g.log.LogErrln("\033[92mAlmost exceeding your drive quota\033[00m")
	case Exceeded:
		g.log.LogErrln("\033[91mThis change will exceed your drive quota\033[00m")
		unSafe = true
	}

	if unSafe {
		unSafeQuotaMsg := fmt.Sprintf("projected size: (%d) %s\n", pushSize, prettyBytes(pushSize))
		if !g.opts.canPrompt() {
			return cannotPromptErr(fmt.Errorf("quota: noPrompt is set yet for quota %s", unSafeQuotaMsg))
		}

		g.log.LogErrf(" %s", unSafeQuotaMsg)
		if status := promptForChanges(); !accepted(status) {
			return status.Error()
		}
	}

	clArg := changeListArg{
		logy:       g.log,
		changes:    nonConflicts,
		noPrompt:   !g.opts.canPrompt(),
		noClobber:  g.opts.NoClobber,
		canPreview: g.opts.canPreview(),
	}

	status, opMap := printChangeList(&clArg)
	if !accepted(status) {
		return status.Error()
	}

	return g.playPushChanges(nonConflicts, opMap)
}

func (g *Commands) resolveConflicts(cl []*Change, push bool) (*[]*Change, *[]*Change) {
	if g.opts.IgnoreConflict {
		return &cl, nil
	}

	nonConflicts, conflicts := sift(cl)
	resolved, unresolved := resolveConflicts(conflicts, push, g.deserializeIndex)
	if conflictsPersist(unresolved) {
		return &resolved, &unresolved
	}

	for _, ch := range unresolved {
		resolved = append(resolved, ch)
	}

	for _, ch := range resolved {
		nonConflicts = append(nonConflicts, ch)
	}
	return &nonConflicts, nil
}

func (g *Commands) PushPiped() (err error) {
	// Cannot push asynchronously because the push order must be maintained
	for _, relToRootPath := range g.opts.Sources {
		rem, resErr := g.rem.FindByPath(relToRootPath)
		if resErr != nil && resErr != ErrPathNotExists {
			return resErr
		}
		if rem != nil && !g.opts.Force {
			return overwriteAttemptedErr(fmt.Errorf("%s already exists remotely, use `%s` to override this behaviour.\n", relToRootPath, ForceKey))
		}

		if hasExportLinks(rem) {
			return googleDocNonExportErr(fmt.Errorf("'%s' is a GoogleDoc/Sheet document cannot be pushed to raw.\n", relToRootPath))
		}

		base := filepath.Base(relToRootPath)
		local := fauxLocalFile(base)
		if rem == nil {
			rem = local
		}

		parentPath := g.parentPather(relToRootPath)
		parent, pErr := g.rem.FindByPath(parentPath)
		if pErr != nil {
			spin := g.playabler()
			spin.play()
			parent, pErr = g.remoteMkdirAll(parentPath)
			spin.stop()
			if pErr != nil || parent == nil {
				g.log.LogErrf("%s: %v\n", relToRootPath, pErr)
				return pErr
			}
		}

		fauxSrc := DupFile(rem)
		if fauxSrc != nil {
			fauxSrc.ModTime = time.Now()
		}

		args := upsertOpt{
			parentId:       parent.Id,
			fsAbsPath:      relToRootPath,
			src:            fauxSrc,
			dest:           rem,
			mask:           g.opts.TypeMask,
			nonStatable:    true,
			ignoreChecksum: g.opts.IgnoreChecksum,
		}

		rem, _, rErr := g.rem.upsertByComparison(os.Stdin, &args)
		if rErr != nil {
			g.log.LogErrf("%s: %v\n", relToRootPath, rErr)
			return rErr
		}

		if rem == nil {
			continue
		}

		index := rem.ToIndex()
		wErr := g.context.SerializeIndex(index)

		// TODO: Should indexing errors be reported?
		if wErr != nil {
			g.log.LogErrf("serializeIndex %s: %v\n", rem.Name, wErr)
		}
	}
	return
}

func (g *Commands) deserializeIndex(identifier string) *config.Index {
	index, err := g.context.DeserializeIndex(identifier)
	if err != nil {
		return nil
	}
	return index
}

func (g *Commands) playPushChanges(cl []*Change, opMap *map[Operation]sizeCounter) (err error) {
	if opMap == nil {
		result := opChangeCount(cl)
		opMap = &result
	}

	totalSize := int64(0)
	ops := *opMap
	for _, counter := range ops {
		totalSize += counter.src
	}

	g.taskStart(totalSize)

	defer close(g.rem.progressChan)

	go func() {
		for n := range g.rem.progressChan {
			g.taskAdd(int64(n))
		}
	}()

	type workPair struct {
		fn  func(*Change) error
		arg *Change
	}

	n := maxProcs()

	sort.Sort(ByPrecedence(cl))

	jobsChan := make(chan semalim.Job)

	go func() {
		defer close(jobsChan)
		throttle := time.Tick(time.Duration(1e9 / n))

		for i, c := range cl {
			if c == nil {
				g.log.LogErrf("BUGON:: push: nil change found for change index %d\n", i)
				continue
			}

			fn := remoteOpToChangerTranslator(g, c)

			if fn == nil {
				g.log.LogErrf("push: cannot find operator for %v", c.Op())
				continue
			}

			cjs := changeJobSt{
				change:   c,
				fn:       fn,
				verb:     "Push",
				throttle: throttle,
			}

			dofner := cjs.changeJober(g)
			jobsChan <- jobSt{id: uint64(i), do: dofner}
		}
	}()

	results := semalim.Run(jobsChan, uint64(n))
	for result := range results {
		res, resErr := result.Value(), result.Err()
		if resErr != nil {
			err = reComposeError(err, fmt.Sprintf("push: %s err: %v\n", res, resErr))
		}
	}

	g.taskFinish()
	return err
}

func lonePush(g *Commands, parent, absPath, path string) (cl, clashes []*Change, err error) {
	remotesChan := g.rem.FindByPathM(absPath)

	var l *File
	localinfo, _ := os.Stat(path)
	if localinfo != nil {
		l = NewLocalFile(path, localinfo)
	}

	iterCount := uint64(0)
	noClashThreshold := uint64(1)

	for r := range remotesChan {
		if r != nil {
			iterCount++
		}

		clr := &changeListResolve{
			remote:    r,
			local:     l,
			push:      true,
			dir:       parent,
			localBase: absPath,
			depth:     g.opts.Depth,
		}

		ccl, cclashes, cErr := g.resolveChangeListRecv(clr)
		cl = append(cl, ccl...)
		clashes = append(clashes, cclashes...)
		if cErr != nil {
			err = combineErrors(err, cErr)
		}
	}

	if iterCount > noClashThreshold && len(clashes) < 1 {
		clashes = append(clashes, cl...)
		err = combineErrors(err, ErrClashesDetected)
	}

	return
}

func (g *Commands) pathSplitter(absPath string) (dir, base string) {
	p := strings.Split(absPath, "/")
	pLen := len(p)
	base = p[pLen-1]
	p = append([]string{"/"}, p[:pLen-1]...)
	dir = gopath.Join(p...)
	return
}

func (g *Commands) parentPather(absPath string) string {
	dir, _ := g.pathSplitter(absPath)
	return dir
}

func (g *Commands) remoteMod(change *Change) (err error) {
	if change.Dest == nil && change.Src == nil {
		err = illogicalStateErr(fmt.Errorf("bug on: both dest and src cannot be nil"))
		g.log.LogErrln(err)
		return err
	}

	absPath := g.context.AbsPathOf(change.Path)

	if change.Src != nil && change.Src.IsDir {
		needsMkdirAll := change.Dest == nil || change.Src.Id == ""
		if needsMkdirAll {
			if destFile, _ := g.remoteMkdirAll(change.Path); destFile != nil {
				change.Src.Id = destFile.Id
			}
		}
	}

	if change.Dest != nil && change.Src != nil && change.Src.Id == "" {
		change.Src.Id = change.Dest.Id // TODO: bad hack
	}

	var parent *File
	parentPath := g.parentPather(change.Path)
	parent, err = g.remoteMkdirAll(parentPath)

	if err != nil {
		g.log.LogErrf("remoteMod/remoteMkdirAll: `%s` got %v\n", parentPath, err)
		return err
	}

	if parent == nil {
		err = errCannotMkdirAll(parentPath)
		g.log.LogErrln(err)
		return
	}

	args := upsertOpt{
		parentId:       parent.Id,
		fsAbsPath:      absPath,
		src:            change.Src,
		dest:           change.Dest,
		mask:           g.opts.TypeMask,
		ignoreChecksum: g.opts.IgnoreChecksum,
		debug:          g.opts.Verbose && g.opts.canPreview(),
	}

	coercedMimeKey, ok := g.coercedMimeKey()
	if ok {
		args.mimeKey = coercedMimeKey
	} else if args.src != nil && !args.src.IsDir { // Infer it from the extension
		args.mimeKey = filepath.Ext(args.src.Name)
	}

	rem, err := g.rem.UpsertByComparison(&args)
	if err != nil {
		g.log.LogErrf("%s: %v\n", change.Path, err)
		return
	}
	if rem == nil {
		return
	}
	index := rem.ToIndex()
	wErr := g.context.SerializeIndex(index)

	// TODO: Should indexing errors be reported?
	if wErr != nil {
		g.log.LogErrf("serializeIndex %s: %v\n", rem.Name, wErr)
	}
	return
}

func (g *Commands) remoteAdd(change *Change) (err error) {
	return g.remoteMod(change)
}

func (g *Commands) remoteUntrash(change *Change) (err error) {
	target := change.Src
	defer func() {
		g.taskAdd(target.Size)
	}()

	err = g.rem.Untrash(target.Id)
	if err != nil {
		return
	}

	index := target.ToIndex()
	wErr := g.context.SerializeIndex(index)

	// TODO: Should indexing errors be reported?
	if wErr != nil {
		g.log.LogErrf("serializeIndex %s: %v\n", target.Name, wErr)
	}
	return
}

func remoteRemover(g *Commands, change *Change, fn func(string) error) (err error) {
	defer func() {
		g.taskAdd(change.Dest.Size)
	}()

	err = fn(change.Dest.Id)
	if err != nil {
		return
	}

	if change.Dest.IsDir {
		mkdirAllMu.Lock()
		g.mkdirAllCache.Remove(change.Path)
		mkdirAllMu.Unlock()
	}

	index := change.Dest.ToIndex()
	err = g.context.RemoveIndex(index, g.context.AbsPathOf(""))

	if err != nil {
		if change.Src != nil {
			g.log.LogErrf("%s \"%s\": remove indexfile %v\n", change.Path, change.Dest.Id, err)
		}
	}
	return
}

func (g *Commands) remoteTrash(change *Change) error {
	return remoteRemover(g, change, g.rem.Trash)
}

func (g *Commands) remoteDelete(change *Change) error {
	return remoteRemover(g, change, g.rem.Delete)
}

func (g *Commands) remoteMkdirAll(d string) (file *File, err error) {
	mkdirAllMu.Lock()

	cachedValue, ok := g.mkdirAllCache.Get(d)

	if ok && cachedValue != nil {
		castF, castOk := cachedValue.Value().(*File)
		// g.log.Logln("CacheHit", d, castF, castOk)
		if castOk && castF != nil {
			mkdirAllMu.Unlock()
			return castF, nil
		}
	}

	// Try the lookup one last time in case a coroutine raced us to it.
	retrFile, retryErr := g.rem.FindByPath(d)

	if retryErr != nil && retryErr != ErrPathNotExists {
		mkdirAllMu.Unlock()
		return retrFile, retryErr
	}

	if retrFile != nil {
		mkdirAllMu.Unlock()
		return retrFile, nil
	}

	parDirPath, last := remotePathSplit(d)

	parent, parentErr := g.rem.FindByPath(parDirPath)

	if parentErr != nil && parentErr != ErrPathNotExists {
		mkdirAllMu.Unlock()
		return parent, parentErr
	}

	mkdirAllMu.Unlock()
	if parent == nil {
		parent, parentErr = g.remoteMkdirAll(parDirPath)
		if parentErr != nil || parent == nil {
			return parent, parentErr
		}
	}

	mkdirAllMu.Lock()
	defer mkdirAllMu.Unlock()

	g.mkdirAllCache.Put(parDirPath, newExpirableCacheValue(parent))

	remoteFile := &File{
		IsDir:   true,
		Name:    last,
		ModTime: time.Now(),
	}

	args := upsertOpt{
		parentId: parent.Id,
		src:      remoteFile,
		debug:    g.opts.Verbose && g.opts.canPreview(),
	}

	cur, curErr := g.rem.UpsertByComparison(&args)
	if curErr != nil {
		return cur, curErr
	}

	if cur == nil {
		return cur, ErrPathNotExists
	}

	index := cur.ToIndex()
	wErr := g.context.SerializeIndex(index)

	// TODO: Should indexing errors be reported?
	if wErr != nil {
		g.log.LogErrf("serializeIndex %s: %v\n", cur.Name, wErr)
	}

	g.mkdirAllCache.Put(d, newExpirableCacheValue(cur))
	return cur, nil
}

func namedPipe(mode os.FileMode) bool {
	return (mode & os.ModeNamedPipe) != 0
}

func symlink(mode os.FileMode) bool {
	return (mode & os.ModeSymlink) != 0
}
