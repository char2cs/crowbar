package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

// surfaces validates design spec 2.5's `surfaces:` block: known surface
// names, a channel the runtime actually speaks, a `terminal` entry backed by
// a real terminal (RuntimeSpec's own structural derivation — HasTerminal is
// never redeclared, only cross-checked), and `start_here` on `terminal`
// keyed on the one thing it promises: that a FRESH spawn can land on that
// surface with no session to name.
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
		// A hooks-channel terminal IS the descriptor's own spawn.cmd PTY,
		// live from the fork and naming no session. An api-channel one is the
		// api transport's attached view, which only runtime.api.attach renders
		// and which names a {session_id} that does not exist yet — rendered at
		// spawn only when the descriptor hotswaps (apiconn.go's
		// applyAPITransport gate), otherwise reached solely through
		// SwitchToTerminal, which needs a completed turn. Hotswap is therefore
		// asked about the api channel ONLY: it governs whether both faces may
		// be live at once, which is a different question from whether a birth
		// can land here.
		if s.StartHere && s.Channel == spec.ChannelAPI && !d.Runtime.Hotswap {
			return invalid(d.ID, "surfaces[terminal]: start_here on channel api requires hotswap — "+
				"attach names a session that does not exist at birth, so a non-hotswap api terminal "+
				"is idle-only sequential handoff")
		}
	}
	return nil
}
