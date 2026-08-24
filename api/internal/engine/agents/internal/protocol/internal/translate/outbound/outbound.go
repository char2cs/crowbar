// Package outbound turns a Crowbar intent into the call a provider expects.
//
// It is the mirror of inbound and the other half of the descriptor's event table: an
// `out:` event names the wire call, and `send:` builds its payload from {braced}
// canonical values. Like inbound it is a pure function — no IO, no state — so the
// whole thing is table-testable.
package outbound

import (
	"sort"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Resolve returns the wire event and its filled payload for a canonical outbound
// event, and whether the provider declares it at all.
//
// Key-presence IS the capability check: a provider that declares no compact_start
// simply cannot be told to compact, and reports false here rather than producing an
// empty call that would reach the wire and confuse it.
func Resolve(
	d *spec.Descriptor,
	canonical string,
	values map[string]string,
) (wireEvent string, send map[string]string, ok bool) {
	e, declared := d.Events[canonical]
	if !declared || e.Out == "" {
		return "", nil, false
	}
	out := make(map[string]string, len(e.Send))
	for field, tmpl := range e.Send {
		out[field] = substitute(tmpl, values)
	}
	return e.Out, out, true
}

// Declared lists the canonical events Crowbar can SEND to this provider, sorted. It is
// what a capability flag is built from.
func Declared(d *spec.Descriptor) []string {
	var out []string
	for name, e := range d.Events {
		if e.Out != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// substitute replaces every {name} with its value. An unsupplied name becomes EMPTY
// rather than being left in place: a provider receiving a literal "{session_id}" is a
// bug that reaches the wire, where an empty field fails loudly at the provider's own
// validation.
func substitute(tmpl string, values map[string]string) string {
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
