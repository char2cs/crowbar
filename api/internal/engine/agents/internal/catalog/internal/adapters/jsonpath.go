package adapters

import (
	"regexp"
	"strings"
)

func selectPath(values []any, path string) []any {
	current := values
	for _, segment := range strings.Split(path, ".") {
		if segment == "" {
			return nil
		}
		expandArray := strings.HasSuffix(segment, "[]")
		key := strings.TrimSuffix(segment, "[]")
		next := make([]any, 0, len(current))
		for _, value := range current {
			selected, ok := descend(value, key)
			if !ok {
				continue
			}
			if expandArray {
				if array, isArray := selected.([]any); isArray {
					next = append(next, array...)
				}
				continue
			}
			next = append(next, selected)
		}
		current = next
	}
	return current
}

func descend(value any, key string) (any, bool) {
	if key == "" {
		return value, true
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	selected, present := object[key]
	return selected, present
}

func lookupField(row map[string]any, path string) any {
	var current any = row
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = object[part]
		if !ok {
			return nil
		}
	}
	return current
}

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
