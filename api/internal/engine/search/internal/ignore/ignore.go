package ignore

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type Stack struct {
	layers []layer
}

type layer struct {
	dir      string
	patterns []pattern
}

type pattern struct {
	raw      string
	negated  bool
	anchored bool
}

func (s *Stack) Add(
	dir string,
	gitignorePath string,
) (err error) {
	f, err := os.Open(gitignorePath) //nolint:gosec // path is controlled by the server
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("ignore: open %s: %w", gitignorePath, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()

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

	if scanErr := scanner.Err(); scanErr != nil {
		return fmt.Errorf("ignore: scan %s: %w", gitignorePath, scanErr)
	}

	s.layers = append(s.layers, layer{dir: dir, patterns: patterns})
	return nil
}

func (s *Stack) ShouldIgnore(
	absPath string,
) bool {
	if containsDotGit(absPath) {
		return true
	}

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

func parseLine(
	line string,
) (pattern, bool) {
	if line == "" || strings.HasPrefix(line, "#") {
		return pattern{}, false
	}

	line = strings.TrimRight(line, " \t\r")
	if line == "" {
		return pattern{}, false
	}

	negated := false
	if strings.HasPrefix(line, "!") {
		negated = true
		line = line[1:]
	} else if strings.HasPrefix(line, `\!`) {
		line = line[1:]
	}

	line = strings.TrimSuffix(line, "/")

	hasLeadingSlash := strings.HasPrefix(line, "/")
	anchored := hasLeadingSlash || strings.Contains(strings.TrimPrefix(line, "/"), "/")
	line = strings.TrimPrefix(line, "/")
	if !doublestar.ValidatePattern(line) {
		return pattern{}, false
	}

	return pattern{
		raw:      line,
		negated:  negated,
		anchored: anchored,
	}, true
}

func matchesPattern(
	p pattern,
	layerDir string,
	absPath string,
) bool {
	rel, err := filepath.Rel(layerDir, absPath)
	if err != nil {
		return false
	}

	if strings.HasPrefix(rel, "..") {
		return false
	}

	rel = filepath.ToSlash(rel)

	if p.anchored {
		return matchesAnchored(p.raw, rel)
	}

	return matchesAnywhere(p.raw, rel)
}

func matchesAnchored(
	raw string,
	rel string,
) bool {
	return doublestar.MatchUnvalidated(raw, rel) || doublestar.MatchUnvalidated(raw+"/**", rel)
}

func matchesAnywhere(
	raw string,
	rel string,
) bool {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if doublestar.MatchUnvalidated(raw, part) {
			return true
		}

		suffix := strings.Join(parts[i:], "/")
		if doublestar.MatchUnvalidated(raw, suffix) {
			return true
		}
	}

	return false
}

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
