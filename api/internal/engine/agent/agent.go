// Package agent defines the AgentRuntime interface, the Hub interface, and
// re-exports ChatFrame types from the internal types package. The concrete
// ACP subprocess runtime is created via New.
package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/session"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/types"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// Re-exported type aliases so callers use the agent.* namespace.
type ChatFrameType = types.ChatFrameType
type ChatFrame = types.ChatFrame

const (
	ChatFrameTypeUserMessage     = types.ChatFrameTypeUserMessage
	ChatFrameTypeAgentChunk      = types.ChatFrameTypeAgentChunk
	ChatFrameTypeAgentTurnEnd    = types.ChatFrameTypeAgentTurnEnd
	ChatFrameTypeToolCall        = types.ChatFrameTypeToolCall
	ChatFrameTypeToolResult      = types.ChatFrameTypeToolResult
	ChatFrameTypeStateTransition = types.ChatFrameTypeStateTransition
)

// Hub registers a chat session and provides the publish/unregister hooks.
// Implementations live in the app layer (app/hub). Defined here so engine
// callers don't need to import app/hub directly.
type Hub = session.Hub

// AgentRuntime is the interface for running an agent inside a flow state.
type AgentRuntime interface {
	Run(
		ctx   context.Context,
		run   domain.AgentRun,
		task  domain.Task,
		state flow.StateDefinition,
	) error
}

// New returns a production AgentRuntime backed by the ACP SDK subprocess protocol.
// runsDirFn receives a run ID and returns the directory where run artifacts are written.
// mcpURL, when non-empty, is forwarded to the ACP session so the subprocess can
// reach the crowbar MCP endpoint and call crowbar_signal. agentCfgDir, when
// non-empty, isolates the agent's Claude configuration from the host user's.
func New(
	cfg config.IntelligenceConfig,
	runsDirFn func(runID string) string,
	hub Hub,
	mcpURL string,
	agentCfgDir string,
) AgentRuntime {
	return session.New(cfg, runsDirFn, hub, mcpURL, agentCfgDir)
}
