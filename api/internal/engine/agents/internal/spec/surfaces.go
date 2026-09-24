package spec

// SurfaceSpec is one VIEW the user can be looking at — Crowbar's own chat, or
// the provider's own terminal/CLI — declared per design spec 2.5 (docs/plans/
// 2026-09-22-descriptor-channel-split.md). Distinct from `channel` (HOW a
// payload arrives: api vs hooks, spec/channel.go — untouched by this) and
// from a capability flag like Hotswap/HasTerminal (WHAT the provider can do):
// a surface names WHERE the user is looking.
type SurfaceSpec struct {
	// Channel is which delivery channel drives this surface's own facts.
	// Validated (rules.surfaces) against RuntimeSpec so a descriptor cannot
	// name a channel it does not actually speak.
	Channel Channel `yaml:"channel"`

	// StartHere is whether a brand-new chat may be launched DIRECTLY onto
	// this surface, rather than reached only by switching to it after spawn
	// (codex's idle-only SwitchToTerminal, runner/attach.go). Defaults false
	// on absence — the same conservative direction as Hotswap/Compaction/
	// every other capability key.
	StartHere bool `yaml:"start_here"`
}

const (
	SurfaceChat     = "chat"
	SurfaceTerminal = "terminal"
)

var SurfaceKinds = map[string]struct{}{
	SurfaceChat:     {},
	SurfaceTerminal: {},
}

// SurfaceStartHere reports whether `name` is a surface this descriptor
// declares a brand-new chat may land on directly. An undeclared surface (or
// one silent on start_here) answers false — absence, not a disabled control.
func (d *Descriptor) SurfaceStartHere(name string) bool {
	s, ok := d.Surfaces[name]
	return ok && s.StartHere
}

// SurfaceChannel is which delivery channel drives `name`'s own facts — the
// declared `surfaces.<name>.channel`, or "" for a surface this descriptor
// does not declare. It is what decides, at spawn, whether a chat landing on
// that surface needs an api connection opened for it at all: a hooks-channel
// surface is fed by the CLI's own PTY, and opening an api connection beside
// it would fork a second session, hide that PTY and drop its hooks as
// redundant echoes of a transport the user is not looking at.
func (d *Descriptor) SurfaceChannel(name string) Channel {
	return d.Surfaces[name].Channel
}
