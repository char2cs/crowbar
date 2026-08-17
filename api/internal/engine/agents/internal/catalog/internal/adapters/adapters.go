// Package adapters reads a provider's catalogue output.
//
// Adapters are named for the SHAPE of the output they parse, never for the
// provider that happens to produce it. That is what keeps a third CLI from
// needing engine code: if its inventory looks like an existing shape, it declares
// that adapter, and if it does not, the new adapter is a new file here rather
// than a branch inside an existing one.
package adapters

import (
	"context"
	"errors"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// ErrMalformedOutput reports provider output the declared adapter cannot read.
var ErrMalformedOutput = errors.New("agents: provider catalogue output is malformed")

// Candidate is one parsed row before the item mapping and normalisation are
// applied. It carries the raw captures only; nothing here is safe to render yet.
type Candidate struct {
	Name        string
	Description string
	Source      string
	ID          string
}

// Result is what an adapter produces: candidates plus any degradation it wants
// reported. Warnings are how a partial answer stays honest instead of silently
// looking complete.
type Result struct {
	Candidates []Candidate
	Warnings   []string
}

// Adapter reads one output shape.
type Adapter interface {
	Probe(ctx context.Context, s *spec.SlashCatalogSpec, runner models.Runner) (Result, error)
}

// For returns the adapter a pipeline selected.
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
