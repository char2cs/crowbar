package outbound

import (
	"encoding/json"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func Resolve(
	d *spec.Descriptor,
	canonical string,
	values map[string]string,
) (wireEvent string, send map[string]string, ok bool) {
	wireEvent, typed, ok := ResolveTyped(d, canonical, values, nil)
	if !ok {
		return "", nil, false
	}
	send = make(map[string]string, len(typed))
	for field, v := range typed {
		if text, isText := v.(string); isText {
			send[field] = text
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return "", nil, false
		}
		send[field] = string(raw)
	}
	return wireEvent, send, true
}

func ResolveTyped(
	d *spec.Descriptor,
	canonical string,
	values map[string]string,
	literals map[string]bool,
) (wireEvent string, send map[string]any, ok bool) {
	e, declared := d.Events[canonical]
	if !declared || e.Out.Empty() {
		return "", nil, false
	}
	send, _ = ExpandTree(e.Send, values, literals).(map[string]any)
	if send == nil {
		send = map[string]any{}
	}
	return e.Out.Name(), send, true
}

func Substitute(tmpl string, values map[string]string) string {
	if !strings.ContainsRune(tmpl, '{') {
		return tmpl
	}
	var b strings.Builder
	rest := tmpl
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			b.WriteString(rest)
			return b.String()
		}
		closeIdx := strings.IndexByte(rest[open:], '}')
		if closeIdx < 0 {
			b.WriteString(rest)
			return b.String()
		}
		closeIdx += open
		b.WriteString(rest[:open])
		b.WriteString(values[rest[open+1:closeIdx]])
		rest = rest[closeIdx+1:]
	}
}

func wholePlaceholder(s string) (string, bool) {
	name, ok := strings.CutPrefix(s, "{")
	if !ok {
		return "", false
	}
	name, ok = strings.CutSuffix(name, "}")
	return name, ok && !strings.ContainsAny(name, "{}")
}

func ExpandTree(node any, values map[string]string, literals map[string]bool) any {
	switch v := node.(type) {
	case string:
		if name, whole := wholePlaceholder(v); whole && literals[name] && values[name] != "" {
			return json.RawMessage(values[name])
		}
		return Substitute(v, values)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = ExpandTree(val, values, literals)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = ExpandTree(val, values, literals)
		}
		return out
	default:
		return v
	}
}
