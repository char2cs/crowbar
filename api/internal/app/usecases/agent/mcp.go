package agent

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	enginemcp "github.com/char2cs/crowbar/api/internal/engine/mcp"
)

func (u *Usecase) DispatchMCP(
	ctx context.Context,
	runnerID, token string,
	message []byte,
) ([]byte, bool, error) {
	if u.minter == nil || u.tools.Resolver == nil {
		return nil, false, fmt.Errorf("agent: dispatch mcp: tool surface not configured")
	}
	tools := agenttools.NewToolSet(u.tools, runnerID, token)
	server := enginemcp.NewServer("crowbar", metadata.GetVersion(), tools)
	out, send := server.Handle(ctx, message)
	return out, send, nil
}
