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

type Runner interface {
	Run(ctx context.Context, argv []string) ([]byte, error)
}
