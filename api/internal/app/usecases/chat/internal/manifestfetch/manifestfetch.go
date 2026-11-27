package manifestfetch

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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
