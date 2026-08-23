package adapters

import (
	"context"
	"errors"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var ErrMalformedOutput = errors.New("agents: provider catalogue output is malformed")

type Candidate struct {
	Name        string
	Description string
	Source      string
	ID          string
}

type Result struct {
	Candidates []Candidate
	Warnings   []string
}

type Adapter interface {
	Probe(ctx context.Context, s *spec.SlashCatalogSpec, runner models.Runner) (Result, error)
}

func For(name string) (Adapter, bool) {
	switch name {
	case spec.CatalogAdapterJSONTextSection:
		return textSection{}, true
	case spec.CatalogAdapterJSONInventoryDetails:
		return inventoryDetails{}, true
	default:
		return nil, false
	}
}
