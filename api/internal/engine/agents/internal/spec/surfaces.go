package spec

type SurfaceSpec struct {
	Channel Channel `yaml:"channel"`

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

func (d *Descriptor) SurfaceStartHere(name string) bool {
	s, ok := d.Surfaces[name]
	return ok && s.StartHere
}

func (d *Descriptor) SurfaceChannel(name string) Channel {
	return d.Surfaces[name].Channel
}
