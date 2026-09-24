package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

// eventOwner validates owner: SPELLING only — api|hooks|either — never which
// events may declare one. Design spec P6b: the descriptor author decides who
// is authoritative; Crowbar's only say is catching a typo.
type eventOwner struct{}

func (eventOwner) Name() string { return "event_owner" }

func (eventOwner) Check(d *spec.Descriptor) error {
	for name, e := range d.Events {
		switch e.Owner {
		case "", spec.OwnerAPI, spec.OwnerHooks, spec.OwnerEither:
		default:
			return invalid(d.ID, "events[%s].owner: must be api, hooks or either, got %q", name, e.Owner)
		}
	}
	return nil
}

// eventSurfaces validates per-event surfaces: SPELLING only — known surface
// names — the same narrow scope as eventOwner. NO CROWBAR-SIDE VETO on WHICH
// events may gate off a surface, even one where the event is the only writer
// of a ledger fact (design spec P6b, "visibility, not veto") — that is
// reported, never rejected; see TestSurfaceGatedEvents_AreReportedLoudly.
type eventSurfaces struct{}

func (eventSurfaces) Name() string { return "event_surfaces" }

func (eventSurfaces) Check(d *spec.Descriptor) error {
	for name, e := range d.Events {
		for _, s := range e.Surfaces {
			if _, ok := spec.SurfaceKinds[s]; !ok {
				return invalid(d.ID, "events[%s].surfaces: unknown surface %q", name, s)
			}
		}
	}
	return nil
}
