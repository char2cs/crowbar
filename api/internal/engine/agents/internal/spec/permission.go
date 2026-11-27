package spec

type PermissionSpec struct {
	Strategy string                         `yaml:"strategy"`
	Levels   map[string]PermissionLevelSpec `yaml:"levels"`
}

type PermissionLevelSpec struct {
	Apply          InjectSteps       `yaml:"apply"`
	Vars           map[string]string `yaml:"vars"`
	VarsAfterLeave map[string]string `yaml:"vars_after_leave"`
}
