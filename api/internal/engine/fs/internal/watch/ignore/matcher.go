// Package ignore is an in-process gitignore matcher used by the file watcher to
// decide, without forking `git check-ignore` per directory, whether a directory
// should be excluded from recursive watching. It honours the repository's root
// .gitignore, nested per-directory .gitignore files, and .git/info/exclude —
// the sources that determine whether a directory tree (node_modules/, dist/,
// vendor/, …) is tracked. It does NOT consult a global core.excludesFile, which
// in practice lists per-file editor/OS noise rather than directories.
package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Matcher decides whether an absolute directory path is gitignored, relative to
// the repository root it was built for.
type Matcher interface {
	// Match reports whether absPath (an absolute directory path under the
	// repository root) is ignored by the repository's gitignore rules.
	Match(
		absPath string,
	) bool
}

// NewMatcher builds a gitignore matcher rooted at repoPath. Gitignore files are
// read lazily and cached on first access, so the matcher does no I/O until the
// first Match call and never re-reads a directory's .gitignore.
func NewMatcher(
	repoPath string,
) Matcher {
	return &matcher{
		repoPath: repoPath,
		dirCache: make(map[string][]pattern),
	}
}

type matcher struct {
	repoPath string
	mu       sync.Mutex
	dirCache map[string][]pattern
	exclude  []pattern
	excLoad  bool
}

func (m *matcher) Match(
	absPath string,
) bool {
	rel, err := filepath.Rel(m.repoPath, absPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}

	ignored := false
	m.eval(m.excludePatterns(), m.repoPath, absPath, &ignored)

	base := m.repoPath
	m.eval(m.dirPatterns(base), base, absPath, &ignored)
	segs := strings.Split(filepath.ToSlash(rel), "/")
	for _, seg := range segs[:len(segs)-1] {
		base = filepath.Join(base, seg)
		m.eval(m.dirPatterns(base), base, absPath, &ignored)
	}
	return ignored
}

// eval applies every pattern in a single gitignore file (in file order, so a
// later line overrides an earlier one) to absPath, updating ignored in place.
func (m *matcher) eval(
	patterns []pattern,
	base string,
	absPath string,
	ignored *bool,
) {
	for _, p := range patterns {
		if p.match(base, absPath) {
			*ignored = !p.negate
		}
	}
}

// dirPatterns returns the compiled patterns from dir/.gitignore, caching the
// result (including the empty/absent case) so each file is read at most once.
func (m *matcher) dirPatterns(
	dir string,
) []pattern {
	m.mu.Lock()
	defer m.mu.Unlock()
	if patterns, ok := m.dirCache[dir]; ok {
		return patterns
	}
	patterns := readPatterns(filepath.Join(dir, ".gitignore"))
	m.dirCache[dir] = patterns
	return patterns
}

// excludePatterns returns the compiled patterns from .git/info/exclude, loaded
// once. It behaves as a lowest-precedence root .gitignore.
func (m *matcher) excludePatterns() []pattern {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.excLoad {
		return m.exclude
	}
	m.exclude = readPatterns(filepath.Join(m.repoPath, ".git", "info", "exclude"))
	m.excLoad = true
	return m.exclude
}

// readPatterns parses every rule line of a gitignore-format file, returning nil
// when the file is absent or empty of rules.
func readPatterns(
	path string,
) []pattern {
	f, err := os.Open(path) //nolint:gosec // G304: path is a fixed .gitignore/exclude under the watched repo
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var patterns []pattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if p, ok := parseLine(scanner.Text()); ok {
			patterns = append(patterns, p)
		}
	}
	return patterns
}
