// Package dispatch resolves an inbound API-transport frame (method plus params) to
// its canonical event name, which the hooks transport receives explicitly but the API
// carries only as a provider method. One method serving several events is split by `when:`.
package dispatch

import (
	"sort"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Resolve finds the canonical event whose in: or ask: names wireMethod and whose
// when: (if any) matches params. Candidates are tried in sorted-name order so an
// ambiguous descriptor still resolves reproducibly.
func Resolve(d *spec.Descriptor, wireMethod string, params map[string]any) (string, bool) {
	names := candidates(d, wireMethod, params, false)
	if len(names) == 0 {
		return "", false
	}
	return names[0], true
}

// Riders are the events declared also: whose in: names wireMethod and whose when:
// matches params, in sorted-name order. They fire beside Resolve's event.
func Riders(d *spec.Descriptor, wireMethod string, params map[string]any) []string {
	return candidates(d, wireMethod, params, true)
}

func candidates(d *spec.Descriptor, wireMethod string, params map[string]any, riders bool) []string {
	names := make([]string, 0, len(d.Events))
	for name := range d.Events {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []string
	for _, name := range names {
		ev := d.Events[name]
		if ev.Also != riders {
			continue
		}
		if !ev.HasChannelBlocks() && d.TransportFor(name) == string(spec.ChannelHooks) {
			continue // a flat event declared on the hook relay never arrives here
		}
		// Only api frames are resolved here: the api: block, or a legacy flat event.
		wire, direction := ev.WireEventFor(spec.ChannelAPI)
		if direction == "out" || !wire.Has(wireMethod) {
			continue
		}
		if !mapping.Match(params, ev.WhenFor(spec.ChannelAPI)) {
			continue
		}
		out = append(out, name)
	}
	return out
}
