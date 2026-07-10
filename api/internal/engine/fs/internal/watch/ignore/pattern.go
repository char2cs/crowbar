package ignore

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// pattern is one compiled line of a gitignore file. glob is the gitignore glob
// with any leading "!" (negation), trailing "/" (directory marker), and leading
// "/" (anchor) stripped; anchored records whether the original line contained an
// internal or leading slash (matched relative to its .gitignore's directory
// rather than at any depth); negate records a leading "!" (re-include).
type pattern struct {
	glob     string
	anchored bool
	negate   bool
}

// parseLine compiles one gitignore line into a pattern. It returns ok=false for
// blank lines and comments, which carry no matching rule.
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
	if trimmed == "" {
		return pattern{}, false
	}
	return pattern{
		glob:     strings.TrimPrefix(trimmed, "/"),
		anchored: strings.Contains(trimmed, "/"),
		negate:   negate,
	}, true
}

// match reports whether absPath is matched by this pattern, where base is the
// directory of the .gitignore the pattern came from. An anchored pattern matches
// the path relative to base; an unanchored pattern matches the path's final
// component at any depth below base.
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
		ok, _ := doublestar.Match(p.glob, rel)
		return ok
	}
	ok, _ := doublestar.Match(p.glob, path.Base(rel))
	return ok
}
