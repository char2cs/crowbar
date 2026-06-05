// Package ignore implements a .gitignore hierarchy loader and path matcher.
package ignore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Stack accumulates .gitignore layers as a directory tree is traversed.
// Patterns from parent directories shadow child directories in the standard
// git precedence order (root first, most-specific last).
//
// The .git/ directory is always ignored regardless of any loaded patterns.
type Stack struct {
	layers []layer
}

type layer struct {
	// dir is the absolute directory this .gitignore applies to.
	dir      string
	patterns []pattern
}

type pattern struct {
	raw     string
	negated bool
	// anchored means the pattern contains a slash (other than trailing),
	// so it is matched relative to the layer's dir only.
	anchored bool
}

// Add loads a .gitignore file at gitignorePath that applies to dir.
// Missing files are silently skipped (not an error).
func (s *Stack) Add(
	dir string,
	gitignorePath string,
) error {
	f, err := os.Open(gitignorePath) //nolint:gosec // path is controlled by the server
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("ignore: open %s: %w", gitignorePath, err)
	}
	defer func() { _ = f.Close() }()

	var patterns []pattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		p, ok := parseLine(line)
		if !ok {
			continue
		}
		patterns = append(patterns, p)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ignore: scan %s: %w", gitignorePath, err)
	}

	s.layers = append(s.layers, layer{dir: dir, patterns: patterns})
	return nil
}

// ShouldIgnore reports whether absPath should be excluded from a walk.
// It always returns true for paths under a .git/ directory.
func (s *Stack) ShouldIgnore(
	absPath string,
) bool {
	if containsDotGit(absPath) {
		return true
	}

	// Later layers (deeper directories) have higher precedence in git.
	// We apply all layers in order; the last matching pattern wins.
	ignored := false
	for _, l := range s.layers {
		for _, p := range l.patterns {
			if matchesPattern(p, l.dir, absPath) {
				ignored = !p.negated
			}
		}
	}

	return ignored
}

// parseLine converts a raw .gitignore line into a pattern.
// Returns (pattern, false) when the line should be skipped.
func parseLine(
	line string,
) (pattern, bool) {
	// Blank lines and comments are ignored.
	if line == "" || strings.HasPrefix(line, "#") {
		return pattern{}, false
	}

	// A backslash-escaped # is a literal hash, not a comment.
	line = strings.TrimRight(line, " \t\r")
	if line == "" {
		return pattern{}, false
	}

	negated := false
	if strings.HasPrefix(line, "!") {
		negated = true
		line = line[1:]
	} else if strings.HasPrefix(line, `\!`) {
		line = line[1:] // unescape leading !
	}

	// A trailing slash means "directory only" — we drop it and match the name.
	line = strings.TrimSuffix(line, "/")

	// Anchored: pattern has a leading slash, or contains a slash in the middle.
	// Both forms pin the pattern to the root of the directory that owns this layer.
	hasLeadingSlash := strings.HasPrefix(line, "/")
	anchored := hasLeadingSlash || strings.Contains(strings.TrimPrefix(line, "/"), "/")
	line = strings.TrimPrefix(line, "/")

	return pattern{
		raw:      line,
		negated:  negated,
		anchored: anchored,
	}, true
}

// matchesPattern reports whether absPath is matched by p, relative to layerDir.
func matchesPattern(
	p pattern,
	layerDir string,
	absPath string,
) bool {
	rel, err := filepath.Rel(layerDir, absPath)
	if err != nil {
		return false
	}

	// Path outside layerDir.
	if strings.HasPrefix(rel, "..") {
		return false
	}

	rel = filepath.ToSlash(rel)

	if p.anchored {
		return matchesAnchored(p.raw, rel)
	}

	return matchesAnywhere(p.raw, rel)
}

// matchesAnchored checks a rooted pattern against the relative path.
// The pattern must match from the repo root; files under a matching directory
// are also covered by appending the doublestar suffix.
func matchesAnchored(
	raw string,
	rel string,
) bool {
	if ok, _ := doublestar.Match(raw, rel); ok {
		return true
	}

	ok, _ := doublestar.Match(raw+"/**", rel)
	return ok
}

// matchesAnywhere checks a non-anchored pattern against each path component
// and every suffix of the relative path.
func matchesAnywhere(
	raw string,
	rel string,
) bool {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if ok, _ := doublestar.Match(raw, part); ok {
			return true
		}

		suffix := strings.Join(parts[i:], "/")
		if ok, _ := doublestar.Match(raw, suffix); ok {
			return true
		}
	}

	return false
}

// containsDotGit reports whether any component of the path is ".git".
func containsDotGit(
	absPath string,
) bool {
	parts := strings.Split(filepath.ToSlash(absPath), "/")
	for _, part := range parts {
		if part == ".git" {
			return true
		}
	}
	return false
}
