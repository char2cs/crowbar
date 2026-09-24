package spec

const EffortFallbackKey = "*"

type EffortSpec struct {
	Available map[string][]string `yaml:"available"`
	Strategy  string              `yaml:"strategy"`
	// Apply is the argv of a forked process; APIApply is the same choice on
	// the api channel — see ModelSpec's own note.
	Apply    []InjectStep `yaml:"apply"`
	APIApply []InjectStep `yaml:"api_apply"`
}
