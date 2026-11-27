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

	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
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

const streamLatency = 0.05

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

	start time.Time

	skipped map[string]bool
	inodes  map[string]inode
}

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
	b.stream = C.watchStreamStart(C.int(w.Fd()), C.CFArrayRef(cfPaths), streamLatency, b.queue)
	if b.stream == nil {
		C.watchQueueRelease(b.queue)
		return nil, errors.Join(errors.New("start FSEvents stream"), r.Close(), w.Close())
	}
	go b.read()
	return b, nil
}

func (b *fseventsBackend) resolveRoots() []string {
	paths := append(append([]string{b.spec.tree}, b.spec.gitDirs...), b.spec.anchors...)
	for _, p := range paths {
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			if p != b.spec.tree {
				continue
			}
			real = safepath.Canonical(p)
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

func (b *fseventsBackend) errors() <-chan error {
	return nil
}

func (b *fseventsBackend) created(context.Context, string) {}

func (b *fseventsBackend) removed(string) {}

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

func (b *fseventsBackend) wanted(
	path string,
) bool {
	if b.spec.file || containsString(b.spec.anchors, path) {
		return true
	}
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
