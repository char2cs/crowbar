// Package outbound turns a Crowbar intent into the call a provider expects, mirroring inbound: an
// `out:` event names the wire call and `send:` builds its payload from {braced} canonical values.
// Pure functions, no IO or state.
package outbound

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Resolve returns the wire event and its filled payload for a canonical outbound
// event, and whether the provider declares it: key-presence is the capability
// check, so an undeclared event never reaches the wire as an empty call.
func Resolve(
	d *spec.Descriptor,
	canonical string,
	values map[string]string,
) (wireEvent string, send map[string]string, ok bool) {
	e, declared := d.Events[canonical]
	if !declared || e.Out.Empty() {
		return "", nil, false
	}
	out := make(map[string]string, len(e.Send))
	for field, tmpl := range e.Send {
		out[field] = Substitute(tmpl, values)
	}
	return e.Out.Name(), out, true
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

// ExpandTree substitutes {placeholder} at every string leaf of a nested payload.
func ExpandTree(node any, values map[string]string) any {
	switch v := node.(type) {
	case string:
		return Substitute(v, values)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = ExpandTree(val, values)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = ExpandTree(val, values)
		}
		return out
	default:
		return v
	}
}
