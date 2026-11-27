package ignore

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type pattern struct {
	glob     string
	anchored bool
	negate   bool
}

func parseLine(
	line string,
) (pattern, bool) {
	line = strings.TrimRight(line, " ")
	if line == "" || strings.HasPrefix(line, "#") {
		return pattern{}, false
	}

	negate := false
	switch {
	case strings.HasPrefix(line, "!"):
		negate = true
		line = line[1:]
	case strings.HasPrefix(line, `\#`), strings.HasPrefix(line, `\!`):
		line = line[1:]
	}

	trimmed := strings.TrimSuffix(line, "/")
	glob := strings.TrimPrefix(trimmed, "/")
	if trimmed == "" || !doublestar.ValidatePattern(glob) {
		return pattern{}, false
	}
	return pattern{
		glob:     glob,
		anchored: strings.Contains(trimmed, "/"),
		negate:   negate,
	}, true
}

func (p pattern) match(
	base string,
	absPath string,
) bool {
	rel, err := filepath.Rel(base, absPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	rel = filepath.ToSlash(rel)
	if p.anchored {
		return doublestar.MatchUnvalidated(p.glob, rel)
	}
	return doublestar.MatchUnvalidated(p.glob, path.Base(rel))
}
