package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// ProviderUsecase owns the agent PROVIDERS: which exist, which are enabled, and the
// MCP surface their CLIs call back into. It is global, never per workspace: a
// preference row is keyed by provider id alone.
type ProviderUsecase interface {
	InstallDescriptor(context.Context, []byte) (descriptorcheck.Report, error)
	// ResolveProviders lists every installed descriptor merged with the user's
	// preference table, ordered by stored priority first and by descriptor id for
	// providers with no row. A provider with no row is enabled.
	ResolveProviders(
		ctx context.Context,
	) ([]domain.AgentProvider, error)

	// DescriptorReports validates every descriptor this machine would load.
	DescriptorReports(
		ctx context.Context,
	) ([]descriptorcheck.Report, error)

	// ReplaceProviderPreferences overwrites the whole preference table and returns the
	// resolved list to broadcast; an omitted provider reverts to its default.
	// problem.ErrInvalidArgument when a row names a provider no descriptor declares.
	ReplaceProviderPreferences(
		ctx context.Context,
		prefs []domain.AgentProviderPreference,
	) ([]domain.AgentProvider, error)

	// DispatchMCP answers one JSON-RPC message against a tool set built PER CALL from
	// the runner's credentials; the bool reports whether to send a response. It refuses
	// rather than advertise an empty tool list when the surface was never configured.
	DispatchMCP(
		ctx context.Context,
		runnerID string,
		token string,
		message []byte,
	) ([]byte, bool, error)

	// VerifyRunnerToken reports whether token is the one minted for runnerID: what
	// a vendor CLI's HTTP hook proves itself with.
	VerifyRunnerToken(runnerID, token string) bool

	// DefaultPermissionLevel is the level a newly created chat is seeded with.
	DefaultPermissionLevel(
		ctx context.Context,
	) (string, error)

	// SetDefaultPermissionLevel overwrites the global default.
	SetDefaultPermissionLevel(
		ctx context.Context,
		level string,
	) error

	// ModelManifestFetchEnabled reports whether a model.manifest: source's
	// background refresh may hit the network. Defaults to true when never
	// set.
	ModelManifestFetchEnabled(
		ctx context.Context,
	) (bool, error)

	// SetModelManifestFetchEnabled overwrites the global toggle.
	SetModelManifestFetchEnabled(
		ctx context.Context,
		enabled bool,
	) error
}

var _ ProviderUsecase = (*Usecase)(nil)

// ErrProviderDisabled is returned when a spawn or switch names a provider the user
// switched OFF, enforced there because a stale tab or the CLI bypasses the listed
// Enabled flag. It wraps problem.ErrInvalidArgument (a 400).
var ErrProviderDisabled = provider.ErrProviderDisabled

func (u *Usecase) InstallDescriptor(ctx context.Context, raw []byte) (descriptorcheck.Report, error) {
	return u.providers.InstallDescriptor(ctx, raw)
}

// ResolveProviders lists every provider this machine offers, in the user's
// order, each with whether it is installed, enabled, and what it can do.
func (u *Usecase) ResolveProviders(
	ctx context.Context,
) ([]domain.AgentProvider, error) {
	return u.providers.ResolveProviders(ctx)
}

// DescriptorReports is every descriptor's static validation, for the provider
// settings: a report with an error is a provider the daemon refuses to enable.
func (u *Usecase) DescriptorReports(
	ctx context.Context,
) ([]descriptorcheck.Report, error) {
	return u.providers.DescriptorReports(ctx)
}

// ReplaceProviderPreferences rewrites the whole preference table and returns the
// resolved list. A provider the submission omits reverts to default rather than
// keeping a stale priority.
func (u *Usecase) ReplaceProviderPreferences(
	ctx context.Context,
	prefs []domain.AgentProviderPreference,
) ([]domain.AgentProvider, error) {
	return u.providers.ReplaceProviderPreferences(ctx, prefs)
}

// DispatchMCP serves one authenticated MCP call from a running vendor CLI.
func (u *Usecase) DispatchMCP(
	ctx context.Context,
	runnerID string,
	token string,
	message []byte,
) ([]byte, bool, error) {
	return u.providers.DispatchMCP(ctx, runnerID, token, message)
}

// VerifyRunnerToken reports whether token was minted for runnerID.
func (u *Usecase) VerifyRunnerToken(
	runnerID string,
	token string,
) bool {
	return u.providers.VerifyRunnerToken(runnerID, token)
}

// providerMCPEnabled reports whether a provider may use the tool surface. It is a
// LIVE port, not read once at spawn, so a Settings change reaches running CLIs.
func (u *Usecase) providerMCPEnabled(
	ctx context.Context,
	providerID string,
) (bool, error) {
	return u.providers.ProviderMCPEnabled(ctx, providerID)
}
