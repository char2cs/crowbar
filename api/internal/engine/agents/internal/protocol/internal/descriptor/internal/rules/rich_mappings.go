package rules

import (
	"sort"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type richMappings struct{}

func (richMappings) Name() string { return "rich event mappings" }

func (richMappings) Check(d *spec.Descriptor) error {
	kinds := map[string]struct{}{
		"read": {}, "edit": {}, "execute": {}, "search": {}, "fetch": {}, "other": {},
	}
	statuses := map[string]struct{}{
		"pending": {}, "active": {}, "running": {}, "completed": {}, "ok": {},
		"failed": {}, "error": {}, "declined": {}, "interrupted": {}, "abandoned": {},
	}
	names := make([]string, 0, len(d.Events))
	for name := range d.Events {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		event := d.Events[name]
		for _, mapped := range event.KindMap {
			if _, ok := kinds[mapped]; !ok {
				return invalid(d.ID, "events[%s].kind_map: unknown canonical kind %q", name, mapped)
			}
		}
		for _, mapped := range event.StatusMap {
			if _, ok := statuses[mapped]; !ok {
				return invalid(d.ID, "events[%s].status_map: unknown canonical status %q", name, mapped)
			}
		}
		if event.Locations != nil && (event.Locations.Items == "" || event.Locations.Path == "") {
			return invalid(d.ID, "events[%s].locations: items and path are required", name)
		}
		if p := event.Patch; p != nil && (p.Items == "" || p.OldStart == "" || p.NewStart == "" || p.Lines == "") {
			return invalid(d.ID, "events[%s].patch: items, old_start, new_start and lines are required", name)
		}
	}
	return nil
}
