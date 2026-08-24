package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ProviderUsecase owns the agent PROVIDERS themselves: which ones exist, which
// the user has enabled, and the MCP surface their CLIs call back into.
//
// It is global, never per workspace: a preference row is keyed by provider id
// alone, so nothing here may depend on a workspace being resolvable.
type ProviderUsecase interface {
	// ResolveProviders lists every installed descriptor merged with the user's
	// preference table, ordered by stored priority first and by descriptor id for
	// providers with no row. A provider with no row is enabled.
	ResolveProviders(
		ctx context.Context,
	) ([]domain.AgentProvider, error)

	// ReplaceProviderPreferences overwrites the whole preference table and
	// returns the resolved list the caller should broadcast. A provider the
	// submission omits reverts to its default rather than keeping a stale
	// priority. Returns apperr.ErrInvalidArgument when a row names a provider no
	// descriptor declares.
	ReplaceProviderPreferences(
		ctx context.Context,
		prefs []domain.AgentProviderPreference,
	) ([]domain.AgentProvider, error)

	// DispatchMCP answers one JSON-RPC message from a runner's CLI against a tool
	// set built PER CALL around that runner's credentials, so no handler can be
	// reached without a successful resolve. The bool reports whether the response
	// is to be sent at all (a notification produces none).
	//
	// It refuses to serve at all when the tool surface was never configured,
	// rather than quietly advertising an empty tool list.
	DispatchMCP(
		ctx context.Context,
		runnerID string,
		token string,
		message []byte,
	) ([]byte, bool, error)
}

var _ ProviderUsecase = (*Usecase)(nil)

// ErrProviderDisabled is returned when a request names an agent provider the
// user has switched OFF in the global preference table.
//
// Disabled is a decision the preference table records and ResolveProviders
// reports, but reporting is not enforcing: a spawn or a provider switch names a
// provider id directly and never passes through the list that Enabled flag
// decorates, so a stale tab, a second window, or the command line would
// otherwise launch a provider the user turned off.
//
// It wraps apperr.ErrInvalidArgument, so handlers answer 400 through the
// existing sentinel mapping with no new case.
var ErrProviderDisabled = provider.ErrProviderDisabled

// The provider table: which vendor CLIs this machine offers, in what order, and
// which of them may call back into Crowbar.
//
// It is global — per user and machine, never per workspace — so nothing here
// takes a workspace id, and the settings route that rewrites it has none to give.

// ResolveProviders lists every provider this machine offers, in the user's
// order, each with whether it is installed, enabled, and what it can do.
func (u *Usecase) ResolveProviders(
	ctx context.Context,
) ([]domain.AgentProvider, error) {
	return u.providers.ResolveProviders(ctx)
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

// providerMCPEnabled reports whether a provider may use the tool surface at all.
// It is bound into the tool Deps as a LIVE port rather than read once at spawn:
// without that, a chat spawned with tools on keeps them for the life of its CLI
// whatever the user does in Settings afterwards.
func (u *Usecase) providerMCPEnabled(
	ctx context.Context,
	providerID string,
) (bool, error) {
	return u.providers.ProviderMCPEnabled(ctx, providerID)
}
