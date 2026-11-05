package ignore

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// pattern is one compiled gitignore line. glob has any leading "!", trailing "/" and
// leading "/" stripped; anchored means the line had a leading or internal slash (match
// relative to its .gitignore's dir); negate means a leading "!".
type pattern struct {
	glob     string
	anchored bool
	negate   bool
}

// parseLine compiles one gitignore line into a pattern. It returns ok=false for
// blank lines, comments and a malformed glob, none of which can match anything.
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

// match reports whether absPath matches, base being the pattern's .gitignore directory:
// an anchored pattern matches the path relative to base, an unanchored one the final
// component at any depth below it.
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
