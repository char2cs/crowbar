// Package mapping is the one path grammar a descriptor writes.
//
// A path is dot-separated. Every accessor takes a LIST of paths, an alternation: the
// first branch resolving to a present, non-empty value wins. A single-path caller
// passes a one-element list.
//
// Every accessor is total: a missing path yields the zero value, never a panic, because
// a descriptor mapping a field a payload does not carry is the normal case.
package mapping

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// patternGroup names the capture group a pattern branch keeps.
const patternGroup = "v"

// A branch is a path read as is, or through one transform. Pattern and Join build the
// transform spellings as one string, so they travel in the same []string as a path.
const (
	patternSep = "\x00"
	joinSep    = "\x01"
)

// Pattern is the branch that keeps the (?P<v>...) group of pattern matched against the
// string at from. A value that is no string, or that pattern does not match, is absent.
func Pattern(from, pattern string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("pattern %q: %w", pattern, err)
	}
	if re.SubexpIndex(patternGroup) < 0 {
		return "", fmt.Errorf("pattern %q: no (?P<%s>...) group", pattern, patternGroup)
	}
	return from + patternSep + pattern, nil
}

// Join is the branch whose value is the present values of paths joined with "/".
func Join(paths ...string) string { return strings.Join(paths, joinSep) }

// LookupKind finds key in kinds, else in each shorter key that drops its last "/part":
// a composite key refines a plain one instead of replacing it.
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

func read(doc map[string]any, branch string) (any, bool) {
	if from, pattern, found := strings.Cut(branch, patternSep); found {
		v, ok := walk(doc, strings.TrimSpace(from))
		return capture(v, ok, pattern)
	}
	if strings.Contains(branch, joinSep) {
		return joined(doc, strings.Split(branch, joinSep))
	}
	return walk(doc, strings.TrimSpace(branch))
}

func capture(v any, ok bool, pattern string) (any, bool) {
	text, isString := v.(string)
	re, err := regexp.Compile(pattern)
	if !ok || !isString || err != nil {
		return nil, false
	}
	match := re.FindStringSubmatch(text)
	if match == nil {
		return nil, false
	}
	return match[re.SubexpIndex(patternGroup)], true
}

func joined(doc map[string]any, paths []string) (any, bool) {
	parts := make([]string, 0, len(paths))
	for _, path := range paths {
		v, _ := walk(doc, strings.TrimSpace(path))
		if text, ok := scalarOf(v); ok && text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return nil, false
	}
	return strings.Join(parts, "/"), true
}

// resolve reads paths against doc. Only alternation skips empty values: the first
// non-empty branch wins, else the first present one, so a lone path keeps "present but empty".
func resolve(doc map[string]any, paths []string) (any, bool) {
	if len(paths) == 1 {
		return read(doc, paths[0])
	}
	var firstPresent any
	var found bool
	for _, branch := range paths {
		v, ok := read(doc, branch)
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
// A whole-key match is tried first so a payload key that itself contains dots — or
// brackets — stays addressable; only then does it descend segment by segment.
//
// A segment of the form `name[field=value]` SELECTS from a list: the first element
// whose field equals value. Real provider payloads put the interesting value inside a
// list (turn.items[type=agentMessage].text), and without selection those are
// unmappable and the provider needs Go.
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

// parseSelector splits a selector segment into its parts. Three shapes:
//
//	name[field=value]  — the first LIST element whose field equals value
//	name[N]            — the Nth LIST element, zero-based
//	name[keypath]      — the MAP entry at the key keypath resolves to
//
// The index form addresses a list element no scalar field match can find, such
// as changes whose `kind` is an object.
//
// The keypath form addresses a MAP keyed by a value the payload names in a
// sibling field (item.states[childIds[0]]); keypath resolves against the object
// name is a field of, not against the map being indexed.
//
// A segment matching none of the three shapes is a plain key.
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

// selectByKey resolves keyPath against sibling — the object the map-valued
// field was itself found on, NOT the map being indexed — to a scalar, then
// looks that scalar up as a key in target. It is how a map keyed by a value
// only the payload itself names (never a literal a descriptor could spell
// out) gets read: see parseSelector's own doc for the motivating shape.
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

// Present reports whether any of paths exists in doc at all — true even for a
// JSON null, false only when every path's key (or an ancestor segment) is
// entirely absent. It exists for callers that must tell "this payload's shape
// never carries this concept" (an api-transport event, say) apart from "it
// carries the concept and it is empty" (a hooks payload naming no
// conversation) — String and the other scalar accessors collapse both into
// the same zero value, which is exactly the distinction that check needs.
func Present(doc map[string]any, paths []string) bool {
	_, ok := resolve(doc, paths)
	return ok
}

func String(doc map[string]any, paths []string) string {
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

func Count(doc map[string]any, paths []string) int {
	v, ok := resolve(doc, paths)
	if !ok {
		return 0
	}
	arr, isArray := v.([]any)
	if !isArray {
		return 0
	}
	return len(arr)
}

func Int(doc map[string]any, paths []string) (int, bool) {
	f, ok := Float(doc, paths)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// Float accepts every numeric shape a decoded payload can carry. JSON numbers arrive
// as float64, but a re-encoded or hand-built map can hold any of these.
func Float(doc map[string]any, paths []string) (float64, bool) {
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
func Bool(doc map[string]any, paths []string) (bool, bool) {
	v, ok := resolve(doc, paths)
	if !ok {
		return false, false
	}
	b, isBool := v.(bool)
	return b, isBool
}

func Time(doc map[string]any, paths []string) (time.Time, bool) {
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
func JSON(doc map[string]any, paths []string) []byte {
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

func Objects(doc map[string]any, paths []string) []map[string]any {
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
func Strings(doc map[string]any, paths []string) []string {
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

func Object(doc map[string]any, paths []string) map[string]any {
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
func Scalar(doc map[string]any, paths []string) (string, bool) {
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

// Match reports whether every when: clause holds. An empty when matches everything, so
// an event that declares none applies unconditionally. Each clause's key is a SINGLE
// discriminator path; its value is the SET of values that path may equal (an `any_of:`
// list, or a one-element list for a plain equality clause).
//
// A clause whose path is missing does NOT match: a variant selector must not silently
// apply to payloads that lack the discriminator.
func Match(doc map[string]any, when map[string][]string) bool {
	for path, want := range when {
		got, ok := Scalar(doc, []string{path})
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
