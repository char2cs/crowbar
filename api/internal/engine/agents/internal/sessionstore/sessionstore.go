// Package sessionstore answers whether a provider's own session exists on
// disk, from the descriptor's session.locate — the check the resume ladder
// makes before it hands a CLI a session id to resume.
package sessionstore

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Finder locates sessions, remembering where each one was last found so a
// repeat check (every restart_tui prompt resumes) is a single stat.
type Finder struct {
	mu     sync.Mutex
	found  map[string]string
	getenv func(string) string
	home   func() (string, error)
}

// New returns a Finder over the process environment.
func New() *Finder {
	return &Finder{found: map[string]string{}, getenv: os.Getenv, home: os.UserHomeDir}
}

// Exists reports whether sessionID exists where loc says the provider keeps
// it. declared is false when loc is nil: the descriptor gives no way to check.
func (f *Finder) Exists(providerID string, loc *spec.SessionLocateSpec, sessionID string) (exists, declared bool) {
	if loc == nil {
		return false, false
	}
	if !safeID(sessionID) {
		return false, true
	}
	key := providerID + "\x00" + sessionID
	if path, ok := f.cached(key); ok {
		if _, err := os.Stat(path); err == nil {
			return true, true
		}
		f.forget(key)
	}
	root, ok := f.root(loc)
	if !ok {
		return false, true
	}
	for _, pattern := range loc.Glob {
		segments := strings.Split(strings.ReplaceAll(pattern, "{id}", sessionID), "/")
		if path, ok := find(root, segments); ok {
			f.remember(key, path)
			return true, true
		}
	}
	return false, true
}

func (f *Finder) root(loc *spec.SessionLocateSpec) (string, bool) {
	root := loc.Root
	if loc.RootEnv != "" {
		if v := f.getenv(loc.RootEnv); v != "" {
			root = v
		}
	}
	if rest, ok := strings.CutPrefix(root, "~/"); ok {
		home, err := f.home()
		if err != nil {
			return "", false
		}
		root = filepath.Join(home, rest)
	}
	return root, root != ""
}

// find walks segments under dir: a literal segment is one stat, a wildcard
// one lists the directory (newest-looking names first, since sessions are
// usually recent and dated directories sort by name).
func find(dir string, segments []string) (string, bool) {
	if len(segments) == 0 {
		return dir, true
	}
	seg, rest := segments[0], segments[1:]
	if !strings.ContainsAny(seg, `*?[`) {
		next := filepath.Join(dir, seg)
		if _, err := os.Stat(next); err != nil {
			return "", false
		}
		return find(next, rest)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	slices.Reverse(entries)
	for _, e := range entries {
		if ok, _ := filepath.Match(seg, e.Name()); !ok {
			continue
		}
		if path, ok := find(filepath.Join(dir, e.Name()), rest); ok {
			return path, true
		}
	}
	return "", false
}

// safeID refuses an id that could step outside the pattern it is placed in.
func safeID(id string) bool {
	return id != "" && len(id) <= 256 && !strings.ContainsAny(id, `/\*?[]`) && !strings.Contains(id, "..")
}

func (f *Finder) cached(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path, ok := f.found[key]
	return path, ok
}

func (f *Finder) remember(key, path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.found[key] = path
}

func (f *Finder) forget(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.found, key)
}
