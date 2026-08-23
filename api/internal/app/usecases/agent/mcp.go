package agent

import (
	"context"
)

// DispatchMCP answers one JSON-RPC message from a runner's CLI. It delegates to
// ProviderUsecase.
func (u *Usecase) DispatchMCP(
	ctx context.Context,
	runnerID, token string,
	message []byte,
) ([]byte, bool, error) {
	return u.providers.DispatchMCP(ctx, runnerID, token, message)
}
