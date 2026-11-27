package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type surfaces struct{}

func (surfaces) Name() string { return "surfaces" }

func (surfaces) Check(d *spec.Descriptor) error {
	for name, s := range d.Surfaces {
		if _, ok := spec.SurfaceKinds[name]; !ok {
			return invalid(d.ID, "surfaces[%s]: unknown surface", name)
		}
		switch s.Channel {
		case spec.ChannelAPI:
			if d.Runtime.API.Protocol == "" {
				return invalid(d.ID, "surfaces[%s]: channel api but runtime declares no api transport", name)
			}
		case spec.ChannelHooks:
			if d.Runtime.Hooks.Format == "" {
				return invalid(d.ID, "surfaces[%s]: channel hooks but runtime declares no hooks wire", name)
			}
		default:
			return invalid(d.ID, "surfaces[%s]: channel must be api or hooks, got %q", name, s.Channel)
		}
		if name != spec.SurfaceTerminal {
			continue
		}
		hasTerminal := d.Runtime.Transport != "api" || len(d.Runtime.API.Attach) > 0
		if !hasTerminal {
			return invalid(d.ID, "surfaces[terminal]: declared but descriptor has no terminal "+
				"(api transport with no attach)")
		}
		if s.StartHere && s.Channel == spec.ChannelAPI && !d.Runtime.Hotswap {
			return invalid(d.ID, "surfaces[terminal]: start_here on channel api requires hotswap — "+
				"attach names a session that does not exist at birth, so a non-hotswap api terminal "+
				"is idle-only sequential handoff")
		}
	}
	return nil
}
