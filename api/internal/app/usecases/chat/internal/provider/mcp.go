package provider

import (
	"context"
	"fmt"

	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	enginemcp "github.com/char2cs/crowbar/api/internal/engine/mcp"
)

func (p *Providers) DispatchMCP(
	ctx context.Context,
	runnerID string,
	token string,
	message []byte,
) ([]byte, bool, error) {
	if p.minter == nil || p.tools.Resolver == nil {
		return nil, false, fmt.Errorf("agent: dispatch mcp: tool surface not configured")
	}
	tools := agenttools.NewToolSet(p.tools, runnerID, token)
	server := enginemcp.NewServer("crowbar", metadata.GetVersion(), tools)
	out, send := server.Handle(ctx, message)
	return out, send, nil
}
