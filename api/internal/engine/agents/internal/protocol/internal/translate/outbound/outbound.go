// Package outbound turns a Crowbar intent into the call a provider expects, mirroring inbound: an
// `out:` event names the wire call and `send:` builds its payload from {braced} canonical values.
// Pure functions, no IO or state.
package outbound

import (
	"encoding/json"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Resolve returns the wire event and its filled payload for a canonical outbound
// event, and whether the provider declares it: key-presence is the capability
// check, so an undeclared event never reaches the wire as an empty call. Every
// value is text; ResolveTyped keeps a number or bool as one.
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
		raw, _ := json.Marshal(v) // a leaf decoded from YAML always marshals
		send[field] = string(raw)
	}
	return wireEvent, send, true
}

// ResolveTyped is Resolve for a wire that carries JSON types. literals names the
// values whose capture was a number or bool: a template that is exactly one such
// {name} sends it as that type, anything else as text.
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

// Substitute replaces every {name} with its value. An unsupplied name becomes EMPTY,
// which the provider's validation rejects loudly, unlike a literal "{session_id}".
// apidriver reuses it at every leaf of nested payloads.
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

// wholePlaceholder reports the name when s is exactly one {name} and nothing else.
func wholePlaceholder(s string) (string, bool) {
	name, ok := strings.CutPrefix(s, "{")
	if !ok {
		return "", false
	}
	name, ok = strings.CutSuffix(name, "}")
	return name, ok && !strings.ContainsAny(name, "{}")
}

// ExpandTree substitutes {placeholder} at every string leaf of a nested payload. A
// leaf that is exactly one {name} listed in literals is that value as raw JSON.
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
