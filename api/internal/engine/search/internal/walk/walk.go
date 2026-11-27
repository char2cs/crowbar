package walk

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

type Ignorer interface {
	ShouldIgnore(absPath string) bool
}

var ErrBadGlob = errors.New("walk: bad glob")

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
	if err := checkGlobs(slices.Concat(includeGlobs, excludeGlobs)); err != nil {
		return nil, err
	}

	out := make(chan string, runtime.GOMAXPROCS(0)*4)

	raw := make(chan string, runtime.GOMAXPROCS(0)*4)

	go func() {
		defer safego.Recover("search.walk.producer")
		defer close(raw)
		err := filepath.WalkDir(repoPath, func(
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
		if err != nil {
			slog.Warn("search: walk: producer stopped early", "repo", repoPath, "err", err)
		}
	}()

	poolSize := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	wg.Add(poolSize)

	for range poolSize {
		go func() {
			defer safego.Recover("search.walk.worker")
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
		defer safego.Recover("search.walk.closer")
		wg.Wait()
		close(out)
	}()

	return out, nil
}

func checkGlobs(
	globs []string,
) error {
	for _, g := range globs {
		if !doublestar.ValidatePattern(g) {
			return fmt.Errorf("walk: %q: %w", g, ErrBadGlob)
		}
	}
	return nil
}

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
		if doublestar.MatchUnvalidated(g, rel) {
			return false
		}
	}

	if len(includeGlobs) == 0 {
		return true
	}

	for _, g := range includeGlobs {
		if doublestar.MatchUnvalidated(g, rel) {
			return true
		}
	}

	return false
}
