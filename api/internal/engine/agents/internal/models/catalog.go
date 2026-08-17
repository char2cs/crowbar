package models

import "context"

// SlashCatalog is an ephemeral, provider-neutral capability result. It contains
// no raw command output, no filesystem locators and no provider config, so it is
// safe to hand straight to a client.
type SlashCatalog struct {
	ProviderID   string
	Completeness string
	Items        []SlashCatalogItem
	Warnings     []string
}

type SlashCatalogItem struct {
	ID          string
	Kind        string
	Label       string
	Description string
	InsertText  string
	Source      string
}

// CatalogItemKindSkill is the only item kind produced today. It is a field
// rather than an assumption so a provider that later exposes commands and skills
// through one inventory does not need the shape changed.
const CatalogItemKindSkill = "skill"

// ProbeOptions supplies only execution context for a bounded provider probe. A
// nil Env means the daemon's own environment.
type ProbeOptions struct {
	Cwd  string
	Env  []string
	Home string
}

// Runner executes one bounded provider command and returns its stdout. It is the
// seam every probe is written against, so a test drives a catalogue without
// spawning a process and production gets the bounded, process-group-isolated
// implementation.
type Runner interface {
	Run(ctx context.Context, argv []string) ([]byte, error)
}
