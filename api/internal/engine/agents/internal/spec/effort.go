package spec

const EffortFallbackKey = "*"

type EffortSpec struct {
	Available map[string][]string `yaml:"available"`
	Strategy  string              `yaml:"strategy"`
	Apply     []InjectStep        `yaml:"apply"`
}
