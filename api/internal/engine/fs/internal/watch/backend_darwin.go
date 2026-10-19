//go:build darwin && cgo

package watch

/*
#cgo LDFLAGS: -framework CoreServices
#include <stdlib.h>
#include "backend_darwin.h"
*/
import "C"

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type op uint32

const (
	opCreate op = 1 << iota
	opWrite
	opRemove
	opRename
	opChmod
)

type event struct {
	Name string
	Op   op
}

// streamLatency is how long FSEvents coalesces after the first event of a
// burst, which NoDefer delivers at once; the watcher debounces on top of it.
const streamLatency = 0.05

// fseventsBackend is one recursive FSEvents stream over the tree and the git
// dirs. It opens no descriptor per file or directory and walks nothing on
// start; it filters events down to what a per-directory watch would see.
//
// The stream's callback writes each event to a pipe that one goroutine reads,
// so no Go code runs on the dispatch thread.
type fseventsBackend struct {
	spec   watchSpec
	roots  []streamRoot
	stream C.FSEventStreamRef
	queue  C.dispatch_queue_t
	r, w   *os.File
	out    chan event
	done   chan struct{}
	exited chan struct{}
	once   sync.Once

	// start is when the stream began; a file born earlier was not created by
	// anything the stream reports.
	start time.Time

	// Owned by the reader goroutine.
	skipped map[string]bool
	inodes  map[string]inode
}

// streamRoot pairs a watched root as the caller spells it with the real path
// FSEvents reports (macOS temp dirs sit behind the /var -> /private/var link).
type streamRoot struct {
	spelled string
	real    string
}

func newBackend(
	_ context.Context,
	spec watchSpec,
) (backend, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("event pipe: %w", err)
	}
	b := &fseventsBackend{
		spec:    spec,
		r:       r,
		w:       w,
		out:     make(chan event, 256),
		done:    make(chan struct{}),
		exited:  make(chan struct{}),
		skipped: make(map[string]bool),
		inodes:  make(map[string]inode),
	}
	cfPaths := C.watchPathsCreate()
	defer C.watchPathsRelease(cfPaths)
	for _, p := range b.resolveRoots() {
		cs := C.CString(p)
		C.watchPathsAppend(cfPaths, cs)
		C.free(unsafe.Pointer(cs))
	}

	b.start = time.Now()
	b.queue = C.watchQueueCreate()
	// Fd puts the write end in blocking mode, which the C writer relies on.
	b.stream = C.watchStreamStart(C.int(w.Fd()), C.CFArrayRef(cfPaths), streamLatency, b.queue)
	if b.stream == nil {
		C.watchQueueRelease(b.queue)
		return nil, errors.Join(errors.New("start FSEvents stream"), r.Close(), w.Close())
	}
	go b.read()
	return b, nil
}

// resolveRoots records every root's real path and returns the outermost ones:
// a stream is recursive, so a main worktree's .git needs no root of its own.
func (b *fseventsBackend) resolveRoots() []string {
	for _, p := range append([]string{b.spec.tree}, b.spec.gitDirs...) {
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			if p != b.spec.tree {
				continue
			}
			real = p
		}
		b.roots = append(b.roots, streamRoot{spelled: p, real: real})
	}
	var outer []string
	for _, r := range b.roots {
		nested := false
		for _, o := range b.roots {
			nested = nested || (o.real != r.real && isUnder(r.real, o.real))
		}
		if !nested && !containsString(outer, r.real) {
			outer = append(outer, r.real)
		}
	}
	return outer
}

func containsString(
	list []string,
	s string,
) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (b *fseventsBackend) events() <-chan event {
	return b.out
}

// errors is nil: a stream reports trouble as MustScanSubDirs events instead.
func (b *fseventsBackend) errors() <-chan error {
	return nil
}

// created and removed are no-ops: the stream is recursive.
func (b *fseventsBackend) created(context.Context, string) {}

func (b *fseventsBackend) removed(string) {}

// close stops the stream, then closes the pipe's write end; the reader keeps draining
// until EOF so a callback blocked on a full pipe can always finish. Later calls return nil.
func (b *fseventsBackend) close() error {
	var err error
	b.once.Do(func() {
		close(b.done)
		C.watchStreamStop(b.stream, b.queue)
		C.watchQueueRelease(b.queue)
		wErr := b.w.Close()
		<-b.exited
		err = errors.Join(wErr, b.r.Close())
	})
	return err
}

// read decodes the pipe's records (flags, path length, path) until EOF, then
// closes the events channel.
func (b *fseventsBackend) read() {
	defer close(b.exited)
	defer close(b.out)
	br := bufio.NewReader(b.r)
	var hdr [8]byte
	for {
		if _, err := io.ReadFull(br, hdr[:]); err != nil {
			return
		}
		path := make([]byte, binary.NativeEndian.Uint32(hdr[4:]))
		if _, err := io.ReadFull(br, path); err != nil {
			return
		}
		b.deliver(string(path), binary.NativeEndian.Uint32(hdr[:4]))
	}
}

func (b *fseventsBackend) deliver(
	path string,
	flags uint32,
) {
	name := b.spelled(path)
	if !b.wanted(name) {
		return
	}
	select {
	case b.out <- event{Name: name, Op: b.opFor(name, flags)}:
	case <-b.done:
	}
}

// spelled maps a reported real path back onto the caller's spelling of the
// root it lies under, so relative paths and git-dir matching keep working.
func (b *fseventsBackend) spelled(
	path string,
) string {
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	best := -1
	for i, r := range b.roots {
		if isUnder(path, r.real) && (best < 0 || len(r.real) > len(b.roots[best].real)) {
			best = i
		}
	}
	if best < 0 {
		return path
	}
	return b.roots[best].spelled + path[len(b.roots[best].real):]
}

// wanted reports whether a per-directory watch over the spec would see path:
// anything in a ref tree, a git dir's direct entries, and working-tree entries
// whose every ancestor directory survives spec.skip.
func (b *fseventsBackend) wanted(
	path string,
) bool {
	for _, root := range b.spec.refTrees {
		if isUnder(path, root) {
			return true
		}
	}
	for _, dir := range b.spec.gitDirs {
		if path == dir || filepath.Dir(path) == dir {
			return true
		}
	}
	if !isUnder(path, b.spec.tree) {
		return false
	}
	if path == b.spec.tree {
		return true
	}
	dir := b.spec.tree
	if b.skipDir(dir) {
		return false
	}
	parent := filepath.Dir(path[len(dir)+1:])
	if parent == "." {
		return true
	}
	for _, seg := range strings.Split(parent, string(filepath.Separator)) {
		dir = filepath.Join(dir, seg)
		if b.skipDir(dir) {
			return false
		}
	}
	return true
}

// skipDir memoises spec.skip; the ignore matcher it consults never re-reads a
// gitignore either, so a cached answer cannot go stale.
func (b *fseventsBackend) skipDir(
	dir string,
) bool {
	skip, ok := b.skipped[dir]
	if !ok {
		skip = b.spec.skip(dir)
		b.skipped[dir] = skip
	}
	return skip
}

// opFor maps a stream's flags onto the fsnotify vocabulary. FSEvents reports a
// path's recent history, not the edge: a file created minutes ago still says
// Created on every write. So a gone path is a removal or a move away, and an
// existing one is created only when this stream first sees that inode there,
// born after the stream started or moved into place.
func (b *fseventsBackend) opFor(
	name string,
	flags uint32,
) op {
	const (
		created  = C.kFSEventStreamEventFlagItemCreated | C.kFSEventStreamEventFlagItemCloned
		removed  = C.kFSEventStreamEventFlagItemRemoved
		renamed  = C.kFSEventStreamEventFlagItemRenamed
		modified = C.kFSEventStreamEventFlagItemModified
		meta     = C.kFSEventStreamEventFlagItemInodeMetaMod | C.kFSEventStreamEventFlagItemXattrMod |
			C.kFSEventStreamEventFlagItemChangeOwner | C.kFSEventStreamEventFlagItemFinderInfoMod
	)
	id, exists := inodeOf(name)
	if !exists {
		delete(b.inodes, name)
		if flags&renamed != 0 && flags&removed == 0 {
			return opRename
		}
		return opRemove
	}
	prev, seen := b.inodes[name]
	b.inodes[name] = id

	var o op
	fresh := !seen || prev != id
	if fresh && (flags&renamed != 0 || (flags&created != 0 && !id.born.Before(b.start))) {
		o |= opCreate
	}
	if flags&modified != 0 {
		o |= opWrite
	}
	if flags&meta != 0 {
		o |= opChmod
	}
	if o == 0 {
		o = opWrite
	}
	return o
}

// inode identifies the file at a path: its number and when it was born.
type inode struct {
	ino  uint64
	born time.Time
}

func inodeOf(
	name string,
) (inode, bool) {
	var st syscall.Stat_t
	if err := syscall.Lstat(name, &st); err != nil {
		return inode{}, false
	}
	return inode{ino: st.Ino, born: time.Unix(st.Birthtimespec.Unix())}, true
}
