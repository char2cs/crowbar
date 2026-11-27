package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

type ProviderUsecase interface {
	InstallDescriptor(context.Context, []byte) (descriptorcheck.Report, error)
	ResolveProviders(
		ctx context.Context,
	) ([]domain.AgentProvider, error)

	DescriptorReports(
		ctx context.Context,
	) ([]descriptorcheck.Report, error)

	ReplaceProviderPreferences(
		ctx context.Context,
		prefs []domain.AgentProviderPreference,
	) ([]domain.AgentProvider, error)

	DispatchMCP(
		ctx context.Context,
		runnerID string,
		token string,
		message []byte,
	) ([]byte, bool, error)

	DispatchRelayMCP(
		ctx context.Context,
		relayID string,
		token string,
		message []byte,
	) ([]byte, bool, error)

	VerifyRunnerToken(runnerID, token string) bool

	DefaultPermissionLevel(
		ctx context.Context,
	) (string, error)

	SetDefaultPermissionLevel(
		ctx context.Context,
		level string,
	) error

	ModelManifestFetchEnabled(
		ctx context.Context,
	) (bool, error)

	SetModelManifestFetchEnabled(
		ctx context.Context,
		enabled bool,
	) error
}

var _ ProviderUsecase = (*Usecase)(nil)

var ErrProviderDisabled = provider.ErrProviderDisabled

func (u *Usecase) InstallDescriptor(ctx context.Context, raw []byte) (descriptorcheck.Report, error) {
	return u.providers.InstallDescriptor(ctx, raw)
}

func (u *Usecase) ResolveProviders(
	ctx context.Context,
) ([]domain.AgentProvider, error) {
	return u.providers.ResolveProviders(ctx)
}

func (u *Usecase) DefaultProviderID(ctx context.Context) (string, error) {
	return u.providers.DefaultProviderID(ctx)
}

func (u *Usecase) DescriptorReports(
	ctx context.Context,
) ([]descriptorcheck.Report, error) {
	return u.providers.DescriptorReports(ctx)
}

func (u *Usecase) ReplaceProviderPreferences(
	ctx context.Context,
	prefs []domain.AgentProviderPreference,
) ([]domain.AgentProvider, error) {
	return u.providers.ReplaceProviderPreferences(ctx, prefs)
}

func (u *Usecase) DispatchMCP(
	ctx context.Context,
	runnerID string,
	token string,
	message []byte,
) ([]byte, bool, error) {
	return u.providers.DispatchMCP(ctx, runnerID, token, message)
}

func (u *Usecase) DispatchRelayMCP(
	ctx context.Context,
	relayID string,
	token string,
	message []byte,
) ([]byte, bool, error) {
	return u.providers.DispatchRelayMCP(ctx, relayID, token, message)
}

func (u *Usecase) VerifyRunnerToken(
	runnerID string,
	token string,
) bool {
	return u.providers.VerifyRunnerToken(runnerID, token)
}

func (u *Usecase) providerMCPEnabled(
	ctx context.Context,
	providerID string,
) (bool, error) {
	return u.providers.ProviderMCPEnabled(ctx, providerID)
}
