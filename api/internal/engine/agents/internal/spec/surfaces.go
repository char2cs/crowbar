package spec

// SurfaceSpec is one VIEW the user can be looking at (Crowbar's chat or the
// provider's terminal): WHERE the user looks, distinct from Channel (how payloads
// arrive) and capability flags (what the provider can do).
type SurfaceSpec struct {
	// Channel is which delivery channel drives this surface's facts, validated
	// (rules.surfaces) against the channels the runtime actually speaks.
	Channel Channel `yaml:"channel"`

	// StartHere is whether a new chat may launch DIRECTLY onto this surface
	// rather than switch to it after spawn. False on absence, like every capability.
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

// SurfaceChannel is `surfaces.<name>.channel`, or "" when undeclared. It decides at
// spawn whether a chat needs an api connection: beside a hooks-channel PTY one would
// fork a second session and drop the PTY's hooks as echoes.
func (d *Descriptor) SurfaceChannel(name string) Channel {
	return d.Surfaces[name].Channel
}
