package models

// SpawnPlan is a fully rendered process launch: what to exec, with what
// environment, in which directory, plus the cleanup for the per-spawn tmp dir.
type SpawnPlan struct {
	// Executable is the resolved absolute path to run. It is carried here rather
	// than left for the caller to re-derive, because the PTY exec resolves argv[0]
	// against the DAEMON's PATH — which under launchd is minimal and misses the
	// directories these CLIs install into.
	Executable string
	Argv       []string
	Env        []string
	Cwd        string
	TmpDir     string
	Cleanup    func()
}

// Display is the agent's user-facing label and glyph. Both may be empty; a
// caller renders the id when they are.
type Display struct {
	Name string
	Icon string
}

// Capabilities reports which optional surfaces an agent declares. Every entry is
// a fact about the descriptor, never a guess: an absent capability renders as
// absent UI, never as a disabled control implying breakage.
type Capabilities struct {
	// PromptSubmit is whether a prompt can be delivered from the chat surface at
	// all. False means terminal-only for that operation.
	PromptSubmit bool
	// Delivery is the declared strategy, empty when PromptSubmit is false.
	Delivery string
	// SlashCatalog is whether a deterministic slash-command inventory exists.
	SlashCatalog bool
	// Telemetry is whether any telemetry transport is declared.
	Telemetry bool
	// ModelSelect and EffortSelect are whether the descriptor declares a model /
	// effort catalogue at all. They are facts about the descriptor, not about any
	// chat: a client reads them to decide whether to render a picker, and reads
	// the catalogue itself to fill it.
	ModelSelect  bool
	EffortSelect bool
	// TerminalPrompts is whether this descriptor declares any blocking-modal
	// needle. False means a screen read could never match, so a caller skips it
	// entirely — the provider's chats then behave byte-identically to how they
	// behaved before terminal-wait detection existed.
	TerminalPrompts bool
	// Observes lists the canonical hook kinds this descriptor maps, sorted. It is
	// what lets the UI say "this agent cannot report tool activity" instead of
	// showing an empty panel that looks broken.
	Observes []string
}

// Observes reports whether the agent maps a canonical hook kind.
func (c Capabilities) Declares(kind string) bool {
	for _, k := range c.Observes {
		if k == kind {
			return true
		}
	}
	return false
}
