package dispatch

import (
	"sort"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func Resolve(d *spec.Descriptor, wireMethod string, params map[string]any) (string, bool) {
	names := candidates(d, wireMethod, params, false)
	if len(names) == 0 {
		return "", false
	}
	return names[0], true
}

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
			continue
		}
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
