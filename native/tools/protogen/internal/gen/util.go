package gen

import (
	"strconv"
	"strings"
	"unicode"
)

// unquoteGo unquotes a Go string literal.
func unquoteGo(
	s string,
) (string, error) {
	return strconv.Unquote(s)
}

// joinPath concatenates a gin group prefix with a relative route path the way
// gin's joinPaths does: the result always starts with "/" and never contains a
// doubled slash, and a group's own trailing slash is not duplicated.
func joinPath(
	prefix string,
	rel string,
) string {
	switch {
	case prefix == "" && rel == "":
		return "/"
	case rel == "":
		return prefix
	}
	out := strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(rel, "/")
	if out == "" {
		return "/"
	}
	return out
}

// toSnake converts a JSON field name to Rust snake_case. Runs of capitals are
// treated as one acronym so "prUrl" becomes "pr_url" and "URLValue" becomes
// "url_value"; digits attach to the run they follow.
func toSnake(
	s string,
) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case r == '-' || r == '.' || r == ' ' || r == '/':
			b.WriteRune('_')
			continue
		case r == '_':
			b.WriteRune('_')
			continue
		}
		if unicode.IsUpper(r) {
			prevLower := i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]))
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			prevUpper := i > 0 && unicode.IsUpper(runes[i-1])
			if i > 0 && (prevLower || (prevUpper && nextLower)) {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return strings.Trim(out, "_")
}

// toCamel converts snake_case back to lowerCamelCase. It is the inverse check
// used to decide whether a struct can carry a single rename_all="camelCase"
// attribute instead of a rename on every field.
func toCamel(
	s string,
) string {
	parts := strings.Split(s, "_")
	var b strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 {
			b.WriteString(p)
			continue
		}
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	return b.String()
}

// toUpperCamel converts an arbitrary identifier to UpperCamelCase, for Rust
// enum variant names derived from wire values like "pr-conflicts".
func toUpperCamel(
	s string,
) string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ' ' || r == '/' || r == ':'
	})
	var b strings.Builder
	for _, f := range fields {
		r := []rune(f)
		if len(r) == 0 {
			continue
		}
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	if b.Len() == 0 {
		return ""
	}
	out := b.String()
	if unicode.IsDigit([]rune(out)[0]) {
		return "V" + out
	}
	return out
}

// rustKeywords are the identifiers a generated Rust field or variant name must
// never collide with. Escaping is by trailing underscore plus an explicit
// serde rename, which avoids raw-identifier edge cases (r#self is illegal).
var rustKeywords = map[string]bool{
	"as": true, "async": true, "await": true, "become": true, "box": true,
	"break": true, "const": true, "continue": true, "crate": true, "do": true,
	"dyn": true, "else": true, "enum": true, "extern": true, "false": true,
	"final": true, "fn": true, "for": true, "gen": true, "if": true,
	"impl": true, "in": true, "let": true, "loop": true, "macro": true,
	"match": true, "mod": true, "move": true, "mut": true, "override": true,
	"priv": true, "pub": true, "ref": true, "return": true, "self": true,
	"static": true, "struct": true, "super": true, "trait": true, "true": true,
	"try": true, "type": true, "typeof": true, "unsafe": true, "unsized": true,
	"use": true, "virtual": true, "where": true, "while": true, "yield": true,
	"Self": true, "abstract": true,
}

// escapeRustIdent makes an identifier safe to emit as a Rust field or variant.
func escapeRustIdent(
	s string,
) string {
	if s == "" {
		return "field_"
	}
	if rustKeywords[s] {
		return s + "_"
	}
	r := []rune(s)
	if unicode.IsDigit(r[0]) {
		return "n" + s
	}
	return s
}

// isTSIdent reports whether a JSON key can be written as a bare TypeScript
// property name rather than a quoted one.
func isTSIdent(
	s string,
) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '_' || r == '$' || unicode.IsLetter(r) {
			continue
		}
		if i > 0 && unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}
