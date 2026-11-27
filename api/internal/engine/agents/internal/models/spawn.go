package models

import "time"

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

	PromptSteer bool

	SlashCatalog bool

	Telemetry bool

	ModelSelect  bool
	EffortSelect bool

	ModelDiscovery bool

	TerminalPrompts bool

	Compaction bool

	ShellStop bool

	Hotswap bool

	HasTerminal bool

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

type PromptTyping struct {
	Input             string
	Submit            string
	ChunkBytes        int
	ChunkInterval     time.Duration
	ComposerRowPrefix string
	ComposerGhost     string
	SkipPrefixes      []string
	SkipSuffixes      []string
}
