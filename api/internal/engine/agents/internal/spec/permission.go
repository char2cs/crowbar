package spec

// PermissionSpec declares, by its map keys, which Crowbar permission levels this
// provider reaches and what each means to it. A level with no key is not offered,
// never clamped to a neighbour.
type PermissionSpec struct {
	Strategy string                         `yaml:"strategy"`
	Levels   map[string]PermissionLevelSpec `yaml:"levels"`
}

// PermissionLevelSpec is how one level is reached: Apply as spawn args, or Vars
// ({permission.<key>}, names the descriptor picks) for transports that send the
// values as request data rather than flags.
type PermissionLevelSpec struct {
	Apply InjectSteps       `yaml:"apply"`
	Vars  map[string]string `yaml:"vars"`
}
