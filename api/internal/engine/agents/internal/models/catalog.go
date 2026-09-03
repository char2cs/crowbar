package models

import "context"

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

const CatalogItemKindSkill = "skill"

type ProbeOptions struct {
	Cwd  string
	Env  []string
	Home string
}

// ProbeRunner executes a one-shot child process for a catalogue or telemetry probe.
// Named for what it does, so `Runner` can mean the live vendor CLI it does not
// describe.
type ProbeRunner interface {
	Run(ctx context.Context, argv []string) ([]byte, error)
}
