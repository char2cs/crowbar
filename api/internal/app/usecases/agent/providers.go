package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ResolveProviders lists every installed provider merged with the user's
// preference table. It delegates to ProviderUsecase.
func (u *Usecase) ResolveProviders(
	ctx context.Context,
) ([]domain.AgentProvider, error) {
	return u.providers.ResolveProviders(ctx)
}

// ReplaceProviderPreferences overwrites the whole provider preference table and
// returns the resolved list. It delegates to ProviderUsecase.
func (u *Usecase) ReplaceProviderPreferences(
	ctx context.Context,
	prefs []domain.AgentProviderPreference,
) ([]domain.AgentProvider, error) {
	return u.providers.ReplaceProviderPreferences(ctx, prefs)
}
