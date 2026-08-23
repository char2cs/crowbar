package normalize

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

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

func Redact(value string) string {
	value = unixPathPattern.ReplaceAllString(value, `${1}[path]`)
	value = homePathPattern.ReplaceAllString(value, `${1}[path]`)
	value = windowsPathPattern.ReplaceAllString(value, `${1}[path]`)
	value = bearerPattern.ReplaceAllString(value, `${1}[redacted]`)
	value = secretPattern.ReplaceAllString(value, `${1}=[redacted]`)
	return openAIKeyPattern.ReplaceAllString(value, `[redacted]`)
}

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

func StripControls(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value)
}

func StripComposerControls(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

func TruncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

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
