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
		if err := checkRichEvent(d.ID, name, d.Events[name], kinds, statuses); err != nil {
			return err
		}
	}
	return nil
}

func checkRichEvent(id, name string, event spec.EventSpec, kinds, statuses map[string]struct{}) error {
	for _, mapped := range event.KindMap {
		if _, ok := kinds[mapped]; !ok {
			return invalid(id, "events[%s].kind_map: unknown canonical kind %q", name, mapped)
		}
	}
	for _, mapped := range event.StatusMap {
		if _, ok := statuses[mapped]; !ok {
			return invalid(id, "events[%s].status_map: unknown canonical status %q", name, mapped)
		}
	}
	if event.Locations != nil && (event.Locations.Items == "" || event.Locations.Path == "") {
		return invalid(id, "events[%s].locations: items and path are required", name)
	}
	if f := event.DiffFiles; f != nil && (f.Items == "" || f.Path == "" || f.Diff == "") {
		return invalid(id, "events[%s].diff_files: items, path and diff are required", name)
	}
	if p := event.Patch; p != nil && (p.Items == "" || p.OldStart == "" || p.NewStart == "" || p.Lines == "") {
		return invalid(id, "events[%s].patch: items, old_start, new_start and lines are required", name)
	}
	return nil
}
