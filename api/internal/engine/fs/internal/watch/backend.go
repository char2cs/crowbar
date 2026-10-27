package watch

import "context"

// watchSpec is what a watcher observes: the working tree minus the directories
// skip prunes, each git dir one level deep, and the ref trees in full. With
// file set, tree is one file, which need not exist, and nothing else is watched.
type watchSpec struct {
	tree     string
	file     bool
	skip     func(dir string) bool
	gitDirs  []string
	refTrees []string
}

// backend delivers the filesystem events inside a watchSpec: inotify through
// fsnotify on Linux, one FSEvents stream on darwin.
type backend interface {
	events() <-chan event
	errors() <-chan error
	// created and removed let a backend that watches directory by directory
	// follow the tree as directories come and go.
	created(ctx context.Context, path string)
	removed(path string)
	close() error
}
