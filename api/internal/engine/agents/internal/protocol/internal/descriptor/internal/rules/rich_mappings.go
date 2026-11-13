package rules

import (
	"slices"
	"sort"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type richMappings struct{}

func (richMappings) Name() string { return "rich event mappings" }

func (richMappings) Check(d *spec.Descriptor) error {
	kinds := map[string]struct{}{
		"read": {}, "edit": {}, "execute": {}, "search": {}, "fetch": {}, "other": {},
		"mcp": {}, "subagent": {}, "plan": {}, "meta": {}, "ask": {},
	}
	statuses := map[string]struct{}{
		"pending": {}, "active": {}, "running": {}, "completed": {}, "ok": {},
		"failed": {}, "error": {}, "declined": {}, "interrupted": {}, "abandoned": {}, "done": {}, "removed": {},
		"cleared": {},
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
	if err := checkCanonicalMaps(id, name, event, kinds, statuses); err != nil {
		return err
	}
	if event.Locations != nil && (event.Locations.Items == "" || event.Locations.Path == "") {
		return invalid(id, "events[%s].locations: items and path are required", name)
	}
	if err := checkDiffFiles(id, name, event.DiffFiles); err != nil {
		return err
	}
	return checkPatch(id, name, event.Patch)
}

func checkPatch(id, name string, p *spec.PatchSpec) error {
	hunks := p != nil && (p.Items != "" || p.OldStart != "" || p.NewStart != "" || p.Lines != "")
	if p != nil && p.Unified != "" && hunks {
		return invalid(id, "events[%s].patch: unified excludes items, old_start, new_start and lines", name)
	}
	if p != nil && p.Unified == "" && (p.Items == "" || p.OldStart == "" || p.NewStart == "" || p.Lines == "") {
		return invalid(id, "events[%s].patch: items, old_start, new_start and lines are required", name)
	}
	return nil
}

func checkDiffFiles(id, name string, f *spec.DiffFilesSpec) error {
	if f == nil {
		return nil
	}
	if f.Items == "" || f.Path == "" || f.Diff == "" {
		return invalid(id, "events[%s].diff_files: items, path and diff are required", name)
	}
	if len(f.Whole) > 0 && f.Kind == "" {
		return invalid(id, "events[%s].diff_files: whole needs kind", name)
	}
	for word, whole := range f.Whole {
		if whole != spec.DiffAdded && whole != spec.DiffDeleted {
			return invalid(id, "events[%s].diff_files.whole[%s]: %q is neither %s nor %s",
				name, word, whole, spec.DiffAdded, spec.DiffDeleted)
		}
	}
	return nil
}

func checkCanonicalMaps(id, name string, event spec.EventSpec, kinds, statuses map[string]struct{}) error {
	for _, mapped := range event.KindMap {
		if _, ok := kinds[mapped]; !ok {
			return invalid(id, "events[%s].kind_map: unknown canonical kind %q", name, mapped)
		}
	}
	for _, mapped := range event.ShellKindMap {
		if mapped != spec.ShellKindShell && mapped != spec.ShellKindMonitor {
			return invalid(id, "events[%s].shell_kind_map: unknown shell kind %q", name, mapped)
		}
	}
	for _, mapped := range event.StatusMap {
		if name == spec.HookTurnFailed {
			if !slices.Contains([]string{spec.FailureAuth, spec.FailureLimit, spec.FailureOverloaded}, mapped) {
				return invalid(id, "events[%s].status_map: unknown failure kind %q", name, mapped)
			}
			continue
		}
		if _, ok := statuses[mapped]; !ok {
			return invalid(id, "events[%s].status_map: unknown canonical status %q", name, mapped)
		}
	}
	return nil
}
