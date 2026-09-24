package adapters

import (
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/pathselect"
)

// selectPath/lookupField delegate to pathselect, which model discovery reads
// the identical grammar through — kept as thin same-signature wrappers so
// this file's own tests (and every existing call site) are untouched.
func selectPath(values []any, path string) []any { return pathselect.Select(values, path) }

func lookupField(row map[string]any, path string) any { return pathselect.Field(row, path) }

func literalSections(text, start, end string) []string {
	sections := []string{}
	for {
		startAt := strings.Index(text, start)
		if startAt < 0 {
			return sections
		}
		text = text[startAt+len(start):]
		endAt := strings.Index(text, end)
		if endAt < 0 {
			return sections
		}
		sections = append(sections, text[:endAt])
		text = text[endAt+len(end):]
	}
}

func namedCaptures(re *regexp.Regexp, match []string) map[string]string {
	out := make(map[string]string, len(match))
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" && i < len(match) {
			out[name] = match[i]
		}
	}
	return out
}

func namedCapturesBytes(re *regexp.Regexp, match [][]byte) map[string]string {
	out := make(map[string]string, len(match))
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" && i < len(match) {
			out[name] = string(match[i])
		}
	}
	return out
}
