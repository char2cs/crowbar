// Package walk provides a concurrent bounded worker-pool directory walker with
// gitignore and glob filtering support.
package walk

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

// Ignorer decides whether an absolute path should be excluded from results.
type Ignorer interface {
	ShouldIgnore(absPath string) bool
}

// Walk returns a channel that receives absolute file paths from repoPath.
//
// Paths are filtered by:
//  1. The ignoreStack (gitignore hierarchy + .git skipping).
//  2. includeGlobs — if non-empty, only paths matching at least one glob pass.
//  3. excludeGlobs — paths matching any glob are dropped.
//
// Globs are matched against the path relative to repoPath using doublestar
// semantics. The channel is closed when the walk completes or ctx is cancelled.
// An error is returned immediately if repoPath is inaccessible.
func Walk(
	ctx context.Context,
	repoPath string,
	ignoreStack Ignorer,
	includeGlobs []string,
	excludeGlobs []string,
) (<-chan string, error) {
	if _, err := os.Lstat(repoPath); err != nil {
		return nil, fmt.Errorf("walk: root inaccessible %s: %w", repoPath, err)
	}

	out := make(chan string, runtime.GOMAXPROCS(0)*4)

	// Raw paths from filepath.WalkDir.
	raw := make(chan string, runtime.GOMAXPROCS(0)*4)

	go func() {
		defer close(raw)
		_ = filepath.WalkDir(repoPath, func(
			path string,
			d fs.DirEntry,
			err error,
		) error {
			if err != nil {
				return nil // skip unreadable entries
			}

			if ignoreStack.ShouldIgnore(path) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if d.IsDir() {
				return nil
			}

			select {
			case <-ctx.Done():
				return filepath.SkipAll
			case raw <- path:
			}
			return nil
		})
	}()

	// Fan-out to a worker pool.
	poolSize := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	wg.Add(poolSize)

	for range poolSize {
		go func() {
			defer wg.Done()
			for path := range raw {
				if !passesGlobs(repoPath, path, includeGlobs, excludeGlobs) {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- path:
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

// passesGlobs reports whether path satisfies the include/exclude glob filters.
func passesGlobs(
	repoPath string,
	absPath string,
	includeGlobs []string,
	excludeGlobs []string,
) bool {
	rel, err := filepath.Rel(repoPath, absPath)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)

	for _, g := range excludeGlobs {
		if matched, _ := doublestar.Match(g, rel); matched {
			return false
		}
	}

	if len(includeGlobs) == 0 {
		return true
	}

	for _, g := range includeGlobs {
		if matched, _ := doublestar.Match(g, rel); matched {
			return true
		}
	}

	return false
}
