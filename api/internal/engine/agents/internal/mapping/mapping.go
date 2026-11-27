package mapping

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

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

func selectSegment(next any, sibling map[string]any, field, want string, dynamic bool) (any, bool) {
	if dynamic {
		return selectByKey(next, sibling, want)
	}
	return selectFrom(next, field, want)
}

const indexField = ""

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

func IsNull(doc map[string]any, paths Expr) bool {
	v, ok := resolve(doc, paths)
	return ok && v == nil
}

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

func Bool(doc map[string]any, paths Expr) (bool, bool) {
	v, ok := resolve(doc, paths)
	if !ok {
		return false, false
	}
	b, isBool := v.(bool)
	return b, isBool
}

func Time(doc map[string]any, paths Expr) (time.Time, bool) {
	if secs, ok := Float(doc, paths); ok {
		return time.Unix(int64(secs), 0).UTC(), true
	}
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

func List(doc map[string]any, paths Expr) []any {
	v, ok := resolve(doc, paths)
	if !ok {
		return nil
	}
	arr, _ := v.([]any)
	return arr
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

func Scalar(doc map[string]any, paths Expr) (string, bool) {
	v, ok := resolve(doc, paths)
	if !ok {
		return "", false
	}
	return scalarOf(v)
}

func Captured(doc map[string]any, paths Expr) (text string, literal, ok bool) {
	v, found := resolve(doc, paths)
	if !found {
		return "", false, false
	}
	text, ok = scalarOf(v)
	switch v.(type) {
	case bool, float32, float64, int, int64, json.Number:
		literal = true
	}
	return text, literal, ok
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
