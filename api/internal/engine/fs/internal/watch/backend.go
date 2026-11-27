package watch

import "context"

type watchSpec struct {
	tree     string
	file     bool
	skip     func(dir string) bool
	gitDirs  []string
	refTrees []string
	anchors  []string
}

type backend interface {
	events() <-chan event
	errors() <-chan error
	created(ctx context.Context, path string)
	removed(path string)
	close() error
}
