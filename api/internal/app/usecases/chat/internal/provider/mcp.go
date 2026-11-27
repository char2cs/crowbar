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
	return p.handle(ctx, agenttools.NewToolSet(p.tools, runnerID, token), message)
}

func (p *Providers) DispatchRelayMCP(
	ctx context.Context,
	relayID string,
	token string,
	message []byte,
) ([]byte, bool, error) {
	if p.minter == nil || p.tools.Resolver == nil {
		return nil, false, fmt.Errorf("agent: dispatch relay mcp: tool surface not configured")
	}
	return p.handle(ctx, agenttools.NewRelayToolSet(p.tools, relayID, token), message)
}

func (p *Providers) handle(ctx context.Context, tools *agenttools.ToolSet, message []byte) ([]byte, bool, error) {
	server := enginemcp.NewServer("crowbar", metadata.GetVersion(), tools)
	out, send := server.Handle(ctx, message)
	return out, send, nil
}

func (p *Providers) VerifyRunnerToken(
	runnerID string,
	token string,
) bool {
	return p.minter != nil && p.minter.Verify(runnerID, token)
}
