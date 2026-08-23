package models

type SpawnPlan struct {
	Executable string
	Argv       []string
	Env        []string
	Cwd        string
	TmpDir     string
	Cleanup    func()
}

type Display struct {
	Name string
	Icon string
}

type Capabilities struct {
	PromptSubmit bool

	Delivery string

	SlashCatalog bool

	Telemetry bool

	ModelSelect  bool
	EffortSelect bool

	TerminalPrompts bool

	Observes []string
}

func (c Capabilities) Declares(kind string) bool {
	for _, k := range c.Observes {
		if k == kind {
			return true
		}
	}
	return false
}
