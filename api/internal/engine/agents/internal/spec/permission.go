package spec

// PermissionSpec declares which of Crowbar's own permission-level names
// (guarded, trusted, full-auto — never enumerated here; the map's own keys
// ARE the declaration) this provider can actually reach, and what each one
// means to it. A level this map has no key for is not offered for this
// provider at all — never clamped to a neighboring one.
type PermissionSpec struct {
	Strategy string                         `yaml:"strategy"`
	Levels   map[string]PermissionLevelSpec `yaml:"levels"`
}

// PermissionLevelSpec is one level's own answer to "what do I actually do
// to get there." Apply is spawn args, exactly like ModelSpec/EffortSpec's
// own Apply. Vars is the same fact under names the descriptor itself picks
// ({permission.<key>}), for a transport that renders a request tree instead
// of an argv — codex's api-transport thread/start send: needs its sandbox
// and approval values as data, not as a flag, and Go must not know sandbox
// or approvalPolicy are Codex's own words for them.
type PermissionLevelSpec struct {
	Apply []InjectStep      `yaml:"apply"`
	Vars  map[string]string `yaml:"vars"`
}
