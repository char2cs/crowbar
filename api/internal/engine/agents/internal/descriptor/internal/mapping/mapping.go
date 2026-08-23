// Package mapping is the one path grammar a descriptor writes.
//
// It replaces two independent resolvers — internal/payload's walk and
// internal/catalog/internal/adapters' selectPath — with a single implementation, so a
// descriptor author learns one syntax and a bug is fixed in one place.
//
// A path is dot-separated. `a || b` is ALTERNATION: the first branch resolving to a
// present, non-empty value wins. Comma is not an operator — v2 overloaded it as
// alternation, which made a comma-bearing key unaddressable.
//
// Every accessor is total: a missing path yields the zero value, never a panic. That
// is deliberate — a descriptor mapping a field a given payload does not carry is the
// normal case, not an error.
package mapping

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const altSep = "||"

// resolve reads expr against doc.
//
// The non-empty rule belongs to ALTERNATION only. A single path returns whatever it
// found, present-but-empty included — Scalar must be able to say "the field is there
// and it is the empty string", which is how the resolver this replaces behaved and
// what its callers depend on. Applying the rule to single paths made an empty leaf
// indistinguishable from a missing one.
//
// Across branches, the first non-empty wins; if every branch that exists is empty, the
// first PRESENT one is returned, so presence still survives.
func resolve(doc map[string]any, expr string) (any, bool) {
	if !strings.Contains(expr, altSep) {
		return walk(doc, strings.TrimSpace(expr))
	}
	var firstPresent any
	var found bool
	for _, branch := range strings.Split(expr, altSep) {
		v, ok := walk(doc, strings.TrimSpace(branch))
		if !ok {
			continue
		}
		if !isEmpty(v) {
			return v, true
		}
		if !found {
			firstPresent, found = v, true
		}
	}
	return firstPresent, found
}

// walk resolves ONE dotted path with no alternation.
//
// A whole-key match is tried first so a payload key that itself contains dots stays
// addressable; only then does it descend segment by segment.
func walk(doc map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	if v, ok := doc[path]; ok {
		return v, true
	}
	var cur any = doc
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

// isEmpty decides what alternation skips over. Only nil and "" count: a false bool and
// a zero number are answers, not absences.
func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	default:
		return false
	}
}

func String(doc map[string]any, expr string) string {
	v, ok := resolve(doc, expr)
	if !ok {
		return ""
	}
	s, isString := v.(string)
	if !isString {
		return ""
	}
	return s
}

func Count(doc map[string]any, expr string) int {
	v, ok := resolve(doc, expr)
	if !ok {
		return 0
	}
	arr, isArray := v.([]any)
	if !isArray {
		return 0
	}
	return len(arr)
}

func Int(doc map[string]any, expr string) (int, bool) {
	f, ok := Float(doc, expr)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// Float accepts every numeric shape a decoded payload can carry. JSON numbers arrive
// as float64, but a re-encoded or hand-built map can hold any of these.
func Float(doc map[string]any, expr string) (float64, bool) {
	v, ok := resolve(doc, expr)
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

// Bool reads a boolean leaf. isEmpty counts only nil and "" as empty, so a `false`
// here is an answer and alternation does not skip past it.
func Bool(doc map[string]any, expr string) (bool, bool) {
	v, ok := resolve(doc, expr)
	if !ok {
		return false, false
	}
	b, isBool := v.(bool)
	return b, isBool
}

func Time(doc map[string]any, expr string) (time.Time, bool) {
	raw := String(doc, expr)
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// JSON returns a leaf as raw bytes: a string leaf verbatim, anything else marshalled.
func JSON(doc map[string]any, expr string) []byte {
	v, ok := resolve(doc, expr)
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

func Objects(doc map[string]any, expr string) []map[string]any {
	v, ok := resolve(doc, expr)
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

func Object(doc map[string]any, expr string) map[string]any {
	v, ok := resolve(doc, expr)
	if !ok {
		return nil
	}
	obj, isObject := v.(map[string]any)
	if !isObject {
		return nil
	}
	return obj
}

// Scalar renders any scalar leaf as text, which is what a catalog row or a template
// substitution needs.
func Scalar(doc map[string]any, expr string) (string, bool) {
	v, ok := resolve(doc, expr)
	if !ok || v == nil {
		return "", false
	}
	switch n := v.(type) {
	case string:
		return n, true
	case bool:
		return strconv.FormatBool(n), true
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(n), 'f', -1, 32), true
	case int:
		return strconv.Itoa(n), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case json.Number:
		return n.String(), true
	default:
		return "", false
	}
}

// Match reports whether every when: clause holds. An empty when matches everything, so
// an event that declares none applies unconditionally.
//
// A clause whose path is missing does NOT match: a variant selector must not silently
// apply to payloads that lack the discriminator.
func Match(doc map[string]any, when map[string]string) bool {
	for path, want := range when {
		got, ok := Scalar(doc, path)
		if !ok || !inAlternation(got, want) {
			return false
		}
	}
	return true
}

func inAlternation(got, want string) bool {
	for _, opt := range strings.Split(want, altSep) {
		if got == strings.TrimSpace(opt) {
			return true
		}
	}
	return false
}
