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

	// Compaction reports whether Crowbar can ASK this provider to compact its
	// context. Key-presence on the descriptor's compact_start event: claude injects
	// its slash command, codex would call thread/compact/start, and a provider that
	// declares neither gets no compact control in the UI.
	Compaction bool

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
