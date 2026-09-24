// Package dispatch resolves an inbound API-transport wire frame — a method name
// plus its decoded params — back to the canonical event name the hooks transport
// already names explicitly (the forwarder CLI is invoked with the canonical name
// as an argument; the API transport carries only the provider's own method name).
//
// Sum types (one wire method serving several canonical events, selected by a
// discriminator field) are resolved via the same `when:` mechanism the
// descriptor already declares for exactly this purpose.
package dispatch

import (
	"sort"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Resolve finds the canonical event whose in: or ask: names wireMethod and whose
// when: (if any) matches params. Deterministic when more than one candidate's
// when: matches (should not happen for a well-formed descriptor, but iteration
// order over a map must not make a bug non-reproducible): candidates are tried in
// sorted-name order and the first match wins.
func Resolve(d *spec.Descriptor, wireMethod string, params map[string]any) (string, bool) {
	names := make([]string, 0, len(d.Events))
	for name := range d.Events {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		ev := d.Events[name]
		if !ev.HasChannelBlocks() && d.TransportFor(name) == string(spec.ChannelHooks) {
			continue // a flat event declared on the hook relay never arrives here
		}
		// The api transport is the only channel this package ever resolves a
		// wire frame FOR — WireEventFor/WhenFor(ChannelAPI) reads a
		// channel-scoped event's own api: block, and falls back to a legacy
		// event's flat in:/when: unchanged.
		wire, direction := ev.WireEventFor(spec.ChannelAPI)
		if direction == "out" || !wire.Has(wireMethod) {
			continue
		}
		if !mapping.Match(params, ev.WhenFor(spec.ChannelAPI)) {
			continue
		}
		return name, true
	}
	return "", false
}
