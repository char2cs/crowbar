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

	// Hotswap is RuntimeSpec.Hotswap, carried through unchanged: whether both of
	// this provider's faces can be live at once.
	Hotswap bool

	// HasTerminal is STRUCTURAL, never declared: true when this descriptor's spawn
	// plan produces a real interactive PTY the user can look at. A hooks-transport
	// provider's PTY IS the vendor CLI, so it always has one; an api-transport
	// provider has one only if it declares `attach` (design spec §3.2 — existence
	// must be derived from what Crowbar was told to run, not declared, because a
	// separate boolean could contradict it).
	HasTerminal bool

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
