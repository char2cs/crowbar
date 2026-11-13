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
	Name   string
	Icon   string
	SignIn string
}

type Capabilities struct {
	PromptSubmit bool

	Delivery string

	// PromptSteer is true when a message sent while a turn runs is delivered
	// into that turn (presentation.prompt_submit.steer) rather than waiting.
	PromptSteer bool

	SlashCatalog bool

	Telemetry bool

	ModelSelect  bool
	EffortSelect bool

	// ModelDiscovery is true when Models()/Efforts() come from a live probe, so
	// validation can tell "not yet resolved" (allow) from "no catalogue" (reject).
	ModelDiscovery bool

	TerminalPrompts bool

	// Compaction reports whether Crowbar can ASK this provider to compact: the
	// descriptor declares compact_start. Without it the UI shows no compact control.
	Compaction bool

	// ShellStop reports whether Crowbar can ASK this provider's agent to end one of
	// its background shells: the descriptor declares shell_stop.
	ShellStop bool

	// Hotswap is RuntimeSpec.Hotswap, carried through unchanged: whether both of
	// this provider's faces can be live at once.
	Hotswap bool

	// HasTerminal is derived, never declared, so it cannot contradict the spawn plan:
	// hooks-transport always has a PTY, api-transport only when it declares `attach`.
	HasTerminal bool

	// TerminalStartHere is `surfaces.terminal.start_here`: a new chat may launch
	// directly onto the terminal surface. False when no terminal surface is declared.
	TerminalStartHere bool

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
