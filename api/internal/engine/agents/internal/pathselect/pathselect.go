// Package pathselect is the one dotted-path/"[]"-expansion walk a decoded
// JSON document is read with — factored out of catalog/internal/adapters so
// model discovery can read the identical grammar without a second
// implementation to drift from the first.
package pathselect

import "strings"

// Select walks path across every value in values. A segment ending in "[]"
// expands: each matched array's elements are flattened into the next step,
// rather than the array itself being carried forward.
func Select(values []any, path string) []any {
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

// Field resolves ONE dotted path (no "[]" expansion) against a single row —
// Select's single-value counterpart, for reading a scalar field off one item.
func Field(row map[string]any, path string) any {
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
