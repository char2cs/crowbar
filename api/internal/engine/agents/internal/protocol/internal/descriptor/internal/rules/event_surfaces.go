package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

// eventSurfaces validates per-event surfaces: SPELLING only — known surface
// names. Which events a descriptor gates off a surface is its own policy.
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
