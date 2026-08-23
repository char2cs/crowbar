package payload

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

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

func Int(p map[string]any, path string) (int, bool) {
	f, ok := Float(p, path)
	if !ok {
		return 0, false
	}
	return int(f), true
}

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

func Bool(p map[string]any, path string) (bool, bool) {
	v, ok := walk(p, path)
	if !ok {
		return false, false
	}
	b, isBool := v.(bool)
	return b, isBool
}

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

func Scalar(p map[string]any, path string) (string, bool) {
	v, ok := walk(p, path)
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
