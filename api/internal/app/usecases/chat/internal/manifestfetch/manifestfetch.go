// Package manifestfetch owns the one global row holding whether Crowbar may refresh
// a model.manifest: catalogue over the network (engine half: SetManifestFetchEnabled).
package manifestfetch

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ErrNoStore is Set's refusal with no backing store: Get degrades to the default,
// but a write fails loudly rather than silently discard the change.
var ErrNoStore = errors.New("agent: model manifest fetch: no store configured")

type ManifestFetch struct {
	prefs store.Store[domain.AgentModelManifestFetch, string]
}

type Deps struct {
	Prefs store.Store[domain.AgentModelManifestFetch, string]
}

func New(d Deps) *ManifestFetch {
	return &ManifestFetch{prefs: d.Prefs}
}

// Get reports whether the network half of model.manifest: resolution is allowed.
// Unset (no row, or no store) reports true, the shipped default.
func (m *ManifestFetch) Get(
	ctx context.Context,
) (bool, error) {
	if m.prefs == nil {
		return true, nil
	}
	row, err := m.prefs.FindByKey(ctx, domain.ModelManifestFetchKey)
	if err != nil {
		return false, fmt.Errorf("agent: model manifest fetch: %w", err)
	}
	if row == nil {
		return true, nil
	}
	return row.Enabled, nil
}

// Set overwrites the global toggle.
func (m *ManifestFetch) Set(
	ctx context.Context,
	enabled bool,
) error {
	if m.prefs == nil {
		return ErrNoStore
	}
	if err := m.prefs.Save(ctx, domain.AgentModelManifestFetch{
		ID: domain.ModelManifestFetchKey, Enabled: enabled,
	}); err != nil {
		return fmt.Errorf("agent: set model manifest fetch: %w", err)
	}
	return nil
}
