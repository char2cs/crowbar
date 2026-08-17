// Package normalize makes provider output safe to show and safe to insert.
//
// Everything here treats provider output as hostile-by-accident rather than
// hostile-by-design: a CLI that prints an absolute path, an API key in an error
// message, or a terminal control sequence is not attacking anyone, but rendering
// any of those verbatim in a chat composer would still be a defect.
package normalize

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Field ceilings. Provider output is bounded before it reaches a UI so a
// malformed inventory cannot produce an unbounded label or an enormous command
// line.
const (
	MaxLabelRunes      = 256
	MaxDescriptionByte = 2 << 10
	MaxInsertTextByte  = 512
	MaxSourceRunes     = 256
	MaxIDBytes         = 512
	MaxWarnings        = 64
	MaxWarningBytes    = 512
)

var (
	unixPathPattern    = regexp.MustCompile(`(^|[\s(])(/(?:[^/\s),;]+/)+[^\s),;]+)`)
	homePathPattern    = regexp.MustCompile(`(^|[\s(])(~/(?:[^/\s),;]+/)*[^\s),;]+)`)
	windowsPathPattern = regexp.MustCompile(`(?i)(^|[\s(])([a-z]:\\(?:[^\\\s),;]+\\)+[^\s),;]+)`)
	bearerPattern      = regexp.MustCompile(`(?i)\b(bearer\s+)[a-z0-9._~+/=-]{8,}`)
	secretPattern      = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\s*[:=]\s*[^\s,;]+`)
	openAIKeyPattern   = regexp.MustCompile(`\bsk-[a-zA-Z0-9_-]{8,}`)
)

// Redact removes filesystem locations and credential-shaped text. Paths go
// because a catalog is published to a client and a home directory leaks the
// user's name; credentials go because a provider that prints one in a diagnostic
// should not have it mirrored into a chat.
func Redact(value string) string {
	value = unixPathPattern.ReplaceAllString(value, `${1}[path]`)
	value = homePathPattern.ReplaceAllString(value, `${1}[path]`)
	value = windowsPathPattern.ReplaceAllString(value, `${1}[path]`)
	value = bearerPattern.ReplaceAllString(value, `${1}[redacted]`)
	value = secretPattern.ReplaceAllString(value, `${1}=[redacted]`)
	return openAIKeyPattern.ReplaceAllString(value, `[redacted]`)
}

// Source normalises a catalog source label. Anything path-shaped is dropped
// entirely rather than redacted: a source is a short name, so a value containing
// a separator is not a name that lost its path — it IS a path.
func Source(value string) string {
	value = strings.TrimSpace(StripComposerControls(value))
	if value == "" || strings.ContainsAny(value, `/\`) || strings.HasPrefix(value, "~") {
		return ""
	}
	if len(value) >= 2 && unicode.IsLetter(rune(value[0])) && value[1] == ':' {
		return ""
	}
	return Redact(value)
}

// StripControls removes control characters that would corrupt a rendered line,
// keeping newline and tab, which are legitimate in a description.
func StripControls(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value)
}

// StripComposerControls removes every control character. Text destined for a
// composer must carry none at all: a newline there would submit the message.
func StripComposerControls(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

// TruncateRunes bounds a value by visible characters.
func TruncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

// TruncateBytes bounds a value by bytes without splitting a rune, so a truncated
// description never ends in a replacement character.
func TruncateBytes(value string, max int) string {
	if len(value) <= max {
		return value
	}
	value = value[:max]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

// Warnings appends additions to existing, redacted, bounded and deduplicated.
func Warnings(existing []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(additions))
	for _, w := range existing {
		seen[w] = struct{}{}
	}
	for _, w := range additions {
		w = TruncateBytes(StripComposerControls(Redact(w)), MaxWarningBytes)
		if w == "" {
			continue
		}
		if _, exists := seen[w]; exists {
			continue
		}
		if len(existing) >= MaxWarnings {
			break
		}
		seen[w] = struct{}{}
		existing = append(existing, w)
	}
	return existing
}
