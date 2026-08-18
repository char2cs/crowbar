package spec

// ModelSpec declares which models a chat may run this CLI under, and how a
// choice reaches the process.
//
// The whole block is optional, and its absence is the capability's absence: a
// provider that declares no models renders no picker, rather than a picker with
// nothing in it.
//
// Available is a FLAT list because a model depends on nothing — unlike an effort
// level, which is a property OF a model (see EffortSpec). It is the DECLARED
// catalogue and the only one: neither shipped CLI exposes an enumeration API, so
// nothing here is discovered at runtime and a stale entry is a descriptor edit
// away rather than a release away.
type ModelSpec struct {
	Available []string     `yaml:"available"`
	Strategy  string       `yaml:"strategy"`
	Apply     []InjectStep `yaml:"apply"`
}
