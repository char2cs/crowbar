package mapping

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const patternGroup = "v"

type Expr []Branch

type Branch struct {
	path  string
	re    *regexp.Regexp
	parts []string
	inner string
}

func Path(path string) Branch { return Branch{path: strings.TrimSpace(path)} }

func Paths(paths ...string) Expr {
	out := make(Expr, len(paths))
	for i, path := range paths {
		out[i] = Path(path)
	}
	return out
}

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

func Join(paths ...string) Branch {
	parts := make([]string, len(paths))
	for i, path := range paths {
		parts[i] = strings.TrimSpace(path)
	}
	return Branch{parts: parts}
}

func Decoded(path, inner string) Branch {
	return Branch{path: strings.TrimSpace(path), inner: strings.TrimSpace(inner)}
}

func (e Expr) Literal() string {
	if len(e) == 0 {
		return ""
	}
	return e[0].path
}

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
	case b.inner != "":
		return b.decoded(doc)
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

func (b Branch) decoded(doc map[string]any) (any, bool) {
	v, ok := walk(doc, b.path)
	if text, isString := v.(string); ok && isString {
		var inner map[string]any
		if json.Unmarshal([]byte(text), &inner) != nil {
			return nil, false
		}
		v = inner
	}
	obj, isObject := v.(map[string]any)
	if !ok || !isObject {
		return nil, false
	}
	return walk(obj, b.inner)
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
