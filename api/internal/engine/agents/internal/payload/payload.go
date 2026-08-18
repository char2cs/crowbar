// Package payload reads typed leaves out of a decoded provider payload by dotted
// path.
//
// Every reader answers "absent" for a path that is missing, that runs through a
// non-object, or whose leaf is the wrong shape. That uniformity is the engine's
// degradation story: a provider that stopped sending a field, a descriptor that
// never mapped it, and an older CLI all land on the same zero value, and none of
// them can fail a hook.
package payload

import (
	"encoding/json"
	"strings"
	"time"
)

// walk descends a dotted path ("a.b.c") and returns the leaf. A bare key is a
// one-segment path. ok is false for any missing or non-object segment.
func walk(p map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	var cur any = p
	for _, seg := range strings.Split(path, ".") {
		m, isObject := cur.(map[string]any)
		if !isObject {
			return nil, false
		}
		next, present := m[seg]
		if !present {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// String returns a string leaf, or "" for anything else.
//
// "Present" must mean a NON-EMPTY STRING wherever this backs a presence check.
// The payload the ownership guard exists to reject spells its absent transcript
// as an explicit JSON null (`"transcript_path": null`), which decodes to a map
// entry that IS there — so a presence-only check would pass it.
func String(p map[string]any, path string) string {
	v, ok := walk(p, path)
	if !ok {
		return ""
	}
	s, isString := v.(string)
	if !isString {
		return ""
	}
	return s
}

// Count returns the length of an array leaf. Anything else — missing, a scalar,
// an object — is 0.
//
// 0 for a path that is not there is the whole safe-degradation story for a level
// a provider re-states: a descriptor that maps nothing and a CLI too old to send
// the field both report "nothing outstanding".
func Count(p map[string]any, path string) int {
	v, ok := walk(p, path)
	if !ok {
		return 0
	}
	arr, isArray := v.([]any)
	if !isArray {
		return 0
	}
	return len(arr)
}

// Int returns a numeric leaf as an int. JSON numbers decode to float64, so both
// are accepted; a fractional value truncates, which is correct for the token and
// duration counts this reads.
func Int(p map[string]any, path string) (int, bool) {
	f, ok := Float(p, path)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// Float returns a numeric leaf. A numeric STRING is deliberately not accepted: a
// provider that quotes its numbers has changed its contract, and silently
// coercing would hide that behind a plausible gauge.
func Float(p map[string]any, path string) (float64, bool) {
	v, ok := walk(p, path)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// Bool returns a boolean leaf.
func Bool(p map[string]any, path string) (bool, bool) {
	v, ok := walk(p, path)
	if !ok {
		return false, false
	}
	b, isBool := v.(bool)
	return b, isBool
}

// Time parses an RFC 3339 string leaf. Anything unparseable is absent rather than
// zero-valued: a reset time of 0001-01-01 rendered as "resets in -2000 years" is
// worse than rendering nothing.
func Time(p map[string]any, path string) (time.Time, bool) {
	raw := String(p, path)
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// JSON returns the leaf re-encoded as JSON bytes, whatever its shape. It is how a
// tool's input and result reach the content store without the engine ever
// learning what a provider's tool arguments mean.
//
// A leaf that is already a string is returned as its raw bytes rather than as a
// quoted JSON string: a provider that reports a tool result as plain text should
// be stored as that text, not as an escaped encoding of it.
func JSON(p map[string]any, path string) []byte {
	v, ok := walk(p, path)
	if !ok || v == nil {
		return nil
	}
	if s, isString := v.(string); isString {
		if s == "" {
			return nil
		}
		return []byte(s)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// Objects returns an array leaf's OBJECT elements, in order, dropping anything in
// it that is not an object.
//
// It is how a descriptor names a list of things — a permission's suggestions, an
// AskUserQuestion's options — without the engine learning what any element means:
// the caller reads each element with the same dotted-path readers it would use on
// a whole payload. A path that is missing, or whose leaf is not an array, yields
// nothing, which is the same absent answer every other reader here gives.
func Objects(p map[string]any, path string) []map[string]any {
	v, ok := walk(p, path)
	if !ok {
		return nil
	}
	arr, isArray := v.([]any)
	if !isArray {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if obj, isObject := item.(map[string]any); isObject {
			out = append(out, obj)
		}
	}
	return out
}

// Object returns an OBJECT leaf, or nil for anything else.
//
// It is the singular of Objects, and it exists for the write side: answering a
// prompt sometimes means handing a provider its own sub-document back with one
// key added, and that needs the subtree as a map rather than as re-encoded bytes.
// The returned map is the decoded payload's own, so a caller that mutates it is
// mutating a value it decoded itself and nobody else holds.
func Object(p map[string]any, path string) map[string]any {
	v, ok := walk(p, path)
	if !ok {
		return nil
	}
	obj, isObject := v.(map[string]any)
	if !isObject {
		return nil
	}
	return obj
}
