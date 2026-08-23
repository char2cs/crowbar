package spec

type ModelSpec struct {
	Available []string     `yaml:"available"`
	Strategy  string       `yaml:"strategy"`
	Apply     []InjectStep `yaml:"apply"`
}
