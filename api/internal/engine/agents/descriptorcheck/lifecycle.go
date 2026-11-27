package descriptorcheck

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var lifecycleEvents = []string{"session_start", "user_prompt", "turn_stop"}

func lifecycleRules(doc document, d *spec.Descriptor) []Finding {
	var out []Finding
	for _, name := range lifecycleEvents {
		ev, ok := d.Events[name]
		if !ok {
			out = append(out, doc.finding("events.lifecycle_missing", SeverityError, "events",
				fmt.Sprintf("no %s event: a chat's turns would never open, close or resume", name),
				fmt.Sprintf("map the CLI's own %s signal under events.%s", name, name)))
			continue
		}
		for _, surface := range sortedKeys(d.Surfaces) {
			channel := d.Surfaces[surface].Channel
			if receivable(ev, channel) {
				continue
			}
			out = append(out, doc.finding("events.channel_missing", SeverityError, "events."+name,
				fmt.Sprintf("the %s surface runs over %s, but %s declares no %s block", surface, channel, name, channel),
				fmt.Sprintf("add events.%s.%s", name, channel)))
		}
	}
	if _, ok := d.Events["permission"]; !ok {
		out = append(out, doc.finding("events.permission_missing", SeverityWarning, "events",
			"no permission event: the CLI's permission asks never reach the chat",
			"map the CLI's permission ask under events.permission"))
	}
	return out
}

func receivable(ev spec.EventSpec, channel spec.Channel) bool {
	if ev.API == nil && ev.Hooks == nil {
		return true
	}
	switch channel {
	case spec.ChannelAPI:
		return ev.API != nil
	case spec.ChannelHooks:
		return ev.Hooks != nil
	default:
		return false
	}
}
