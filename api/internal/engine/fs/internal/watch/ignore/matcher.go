package ignore

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Matcher interface {
	Match(
		absPath string,
	) bool
}

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

func readPatterns(
	path string,
) []pattern {
	f, err := os.Open(path) //nolint:gosec // G304: path is a fixed .gitignore/exclude under the watched repo
	if err != nil {
		return nil
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("watch: ignore: close", "path", path, "err", err)
		}
	}()

	var patterns []pattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if p, ok := parseLine(scanner.Text()); ok {
			patterns = append(patterns, p)
		}
	}
	return patterns
}
