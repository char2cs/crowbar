// Package mapping is the one dot-separated path grammar a descriptor writes. Every accessor takes
// an Expr whose first present, non-empty branch wins, and is total (a missing path yields the zero
// value): a descriptor mapping a field a payload does not carry is the normal case.
package mapping

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// resolve reads paths against doc. Only alternation skips empty values: the first
// non-empty branch wins, else the first present one, so a lone path keeps "present but empty".
func resolve(doc map[string]any, paths Expr) (any, bool) {
	if len(paths) == 1 {
		return paths[0].read(doc)
	}
	var firstPresent any
	var found bool
	for _, branch := range paths {
		v, ok := branch.read(doc)
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

// walk resolves ONE dotted path with no alternation. A whole-key match is tried
// first so a key containing dots or brackets stays addressable; selector segments
// (see parseSelector) let a descriptor reach values payloads keep inside lists or maps.
func walk(doc map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	if v, ok := doc[path]; ok {
		return v, true
	}
	var cur any = doc
	for _, seg := range strings.Split(path, ".") {
		name, field, want, dynamic, isSelector := parseSelector(seg)

		m, isObject := cur.(map[string]any)
		if !isObject {
			return nil, false
		}
		next, present := m[name]
		if !present {
			return nil, false
		}
		if !isSelector {
			cur = next
			continue
		}

		picked, ok := selectSegment(next, m, field, want, dynamic)
		if !ok {
			return nil, false
		}
		cur = picked
	}
	return cur, true
}

// selectSegment dispatches a selector segment's already-parsed pieces to the
// select function that shape needs: dynamic key lookups reach into a MAP,
// everything else reaches into a LIST.
func selectSegment(next any, sibling map[string]any, field, want string, dynamic bool) (any, bool) {
	if dynamic {
		return selectByKey(next, sibling, want)
	}
	return selectFrom(next, field, want)
}

// indexField is the sentinel parseSelector reports for `name[N]`. It is not a
// legal payload key — a JSON object key could be "0", but never "" — so it can
// never collide with a real `name[field=value]` selector.
const indexField = ""

// parseSelector splits a selector segment: name[field=value] (first list element
// whose field equals value), name[N] (Nth list element), or name[keypath] (map entry
// at the key keypath resolves to, against the sibling object). Anything else is a plain key.
func parseSelector(seg string) (name, field, want string, dynamic, ok bool) {
	open := strings.IndexByte(seg, '[')
	if open <= 0 || !strings.HasSuffix(seg, "]") {
		return seg, "", "", false, false
	}
	inner := seg[open+1 : len(seg)-1]
	if field, want, found := strings.Cut(inner, "="); found && field != "" {
		return seg[:open], field, want, false, true
	}
	if inner == "" || strings.Contains(inner, "=") {
		return seg, "", "", false, false
	}
	if strings.IndexFunc(inner, func(r rune) bool { return r < '0' || r > '9' }) < 0 {
		return seg[:open], indexField, inner, false, true
	}
	return seg[:open], indexField, inner, true, true
}

// selectFrom picks one element out of list: by zero-based index when field is
// indexField, otherwise the first element whose field equals want.
func selectFrom(list any, field, want string) (any, bool) {
	arr, isArray := list.([]any)
	if !isArray {
		return nil, false
	}
	if field == indexField {
		i, err := strconv.Atoi(want)
		if err != nil || i < 0 || i >= len(arr) {
			return nil, false
		}
		return arr[i], true
	}
	for _, item := range arr {
		obj, isObject := item.(map[string]any)
		if !isObject {
			continue
		}
		if got, ok := scalarOf(obj[field]); ok && got == want {
			return obj, true
		}
	}
	return nil, false
}

// selectByKey resolves keyPath against sibling (the object holding the map, NOT the
// map itself) to a scalar and looks it up in target, for maps keyed by a value only
// the payload names.
func selectByKey(target any, sibling map[string]any, keyPath string) (any, bool) {
	obj, isObject := target.(map[string]any)
	if !isObject {
		return nil, false
	}
	resolved, ok := walk(sibling, keyPath)
	if !ok {
		return nil, false
	}
	key, ok := scalarOf(resolved)
	if !ok {
		return nil, false
	}
	v, present := obj[key]
	return v, present
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

// Present reports whether any of paths exists in doc, even as JSON null. It tells
// "this payload never carries the concept" apart from "carries it empty", which the
// scalar accessors collapse into the same zero value.
func Present(doc map[string]any, paths Expr) bool {
	_, ok := resolve(doc, paths)
	return ok
}

func String(doc map[string]any, paths Expr) string {
	v, ok := resolve(doc, paths)
	if !ok {
		return ""
	}
	s, isString := v.(string)
	if !isString {
		return ""
	}
	return s
}

func Int(doc map[string]any, paths Expr) (int, bool) {
	f, ok := Float(doc, paths)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// Float accepts every numeric shape a decoded payload can carry. JSON numbers arrive
// as float64, but a re-encoded or hand-built map can hold any of these.
func Float(doc map[string]any, paths Expr) (float64, bool) {
	v, ok := resolve(doc, paths)
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
func Bool(doc map[string]any, paths Expr) (bool, bool) {
	v, ok := resolve(doc, paths)
	if !ok {
		return false, false
	}
	b, isBool := v.(bool)
	return b, isBool
}

func Time(doc map[string]any, paths Expr) (time.Time, bool) {
	raw := String(doc, paths)
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
func JSON(doc map[string]any, paths Expr) []byte {
	v, ok := resolve(doc, paths)
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

func Objects(doc map[string]any, paths Expr) []map[string]any {
	v, ok := resolve(doc, paths)
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

// Strings reads an array of strings; non-string elements are skipped.
func Strings(doc map[string]any, paths Expr) []string {
	v, ok := resolve(doc, paths)
	if !ok {
		return nil
	}
	arr, isArray := v.([]any)
	if !isArray {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, isString := item.(string); isString {
			out = append(out, s)
		}
	}
	return out
}

func Object(doc map[string]any, paths Expr) map[string]any {
	v, ok := resolve(doc, paths)
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
func Scalar(doc map[string]any, paths Expr) (string, bool) {
	v, ok := resolve(doc, paths)
	if !ok {
		return "", false
	}
	return scalarOf(v)
}

func scalarOf(v any) (string, bool) {
	if v == nil {
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

// Match reports whether every when: clause (one discriminator path -> the set of values
// it may equal) holds; an empty when matches everything. A missing path does NOT match,
// so a variant never silently applies to payloads lacking the discriminator.
func Match(doc map[string]any, when map[string][]string) bool {
	for path, want := range when {
		got, ok := Scalar(doc, Paths(path))
		if !ok || !inSet(got, want) {
			return false
		}
	}
	return true
}

func inSet(got string, want []string) bool {
	for _, opt := range want {
		if got == strings.TrimSpace(opt) {
			return true
		}
	}
	return false
}
