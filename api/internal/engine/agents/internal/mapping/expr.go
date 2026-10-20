package mapping

import (
	"fmt"
	"regexp"
	"strings"
)

// patternGroup names the capture group a pattern branch keeps.
const patternGroup = "v"

// Expr is the alternatives of one mapped field, in the order a descriptor lists them.
type Expr []Branch

// Branch is one alternative: a path read as is, or a value computed from the payload.
// A pattern is compiled when it is built, so reading a payload never compiles anything.
type Branch struct {
	path  string
	re    *regexp.Regexp
	parts []string
}

// Path is the branch that reads the dotted path as is.
func Path(path string) Branch { return Branch{path: strings.TrimSpace(path)} }

// Paths is the Expr of plain path branches.
func Paths(paths ...string) Expr {
	out := make(Expr, len(paths))
	for i, path := range paths {
		out[i] = Path(path)
	}
	return out
}

// Pattern is the branch that keeps the (?P<v>...) group of pattern matched against the
// string at from. A value that is no string, or that pattern does not match, is absent.
func Pattern(from, pattern string) (Branch, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return Branch{}, fmt.Errorf("pattern %q: %w", pattern, err)
	}
	if re.SubexpIndex(patternGroup) < 0 {
		return Branch{}, fmt.Errorf("pattern %q: no (?P<%s>...) group", pattern, patternGroup)
	}
	return Branch{path: strings.TrimSpace(from), re: re}, nil
}

// Join is the branch whose value is the present, non-empty values of paths joined with
// "/"; a missing part is skipped, so a leading gap leaves the bare remainder.
func Join(paths ...string) Branch {
	parts := make([]string, len(paths))
	for i, path := range paths {
		parts[i] = strings.TrimSpace(path)
	}
	return Branch{parts: parts}
}

// Literal is the first branch's text as the descriptor wrote it, for a field whose value
// is a constant rather than a payload path.
func (e Expr) Literal() string {
	if len(e) == 0 {
		return ""
	}
	return e[0].path
}

// LookupKind finds key in kinds, else in each shorter key that drops its last "/part":
// a composite key refines a plain one instead of replacing it.
func LookupKind(kinds map[string]string, key string) (string, bool) {
	for {
		if kind, ok := kinds[key]; ok {
			return kind, true
		}
		cut := strings.LastIndexByte(key, '/')
		if cut < 0 {
			return "", false
		}
		key = key[:cut]
	}
}

func (b Branch) read(doc map[string]any) (any, bool) {
	switch {
	case b.re != nil:
		return b.capture(doc)
	case b.parts != nil:
		return b.joined(doc)
	}
	return walk(doc, b.path)
}

func (b Branch) capture(doc map[string]any) (any, bool) {
	v, ok := walk(doc, b.path)
	text, isString := v.(string)
	if !ok || !isString {
		return nil, false
	}
	match := b.re.FindStringSubmatch(text)
	if match == nil {
		return nil, false
	}
	return match[b.re.SubexpIndex(patternGroup)], true
}

func (b Branch) joined(doc map[string]any) (any, bool) {
	parts := make([]string, 0, len(b.parts))
	for _, path := range b.parts {
		v, _ := walk(doc, path)
		if text, ok := scalarOf(v); ok && text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return nil, false
	}
	return strings.Join(parts, "/"), true
}
