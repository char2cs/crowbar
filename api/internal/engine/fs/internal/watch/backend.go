package watch

// watchSpec is what a watcher observes: the working tree minus the directories
// skip prunes, each git dir one level deep, and the ref trees in full.
type watchSpec struct {
	tree     string
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
	created(path string)
	removed(path string)
	close() error
}
