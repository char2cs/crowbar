package engine

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// Container holds all engine-layer dependencies.
type Container struct {
	FlowLoader   flow.Loader
	AgentRuntime agent.AgentRuntime
}

type engineOpts struct {
	homeDir      string
	agentRuntime agent.AgentRuntime
	mcpURL       string
}

// Option configures engine.New.
type Option func(*engineOpts)

// WithHomeDir overrides the home directory used for path resolution,
// bypassing the process-level HOME env var. Intended for test isolation.
func WithHomeDir(dir string) Option {
	return func(o *engineOpts) { o.homeDir = dir }
}

// WithAgentRuntime injects a concrete AgentRuntime implementation,
// overriding the default ACP-based runtime. Intended for test isolation.
func WithAgentRuntime(rt agent.AgentRuntime) Option {
	return func(o *engineOpts) { o.agentRuntime = rt }
}

// WithMCPURL sets the URL of the crowbar MCP server that the ACP subprocess
// agent will use to call crowbar_signal. Must be a regular TCP URL because the
// agent subprocess cannot use the kit's Unix-socket dialer.
func WithMCPURL(url string) Option {
	return func(o *engineOpts) { o.mcpURL = url }
}

func runsDir(base, id string) string {
	if base == "" {
		return id
	}
	return filepath.Join(base, id)
}

// New constructs the engine container. If WithAgentRuntime is not provided,
// a real ACP subprocess runtime is constructed when hub is non-nil.
func New(
	_ context.Context,
	hub agent.Hub,
	opts ...Option,
) (*Container, error) {
	cfg := engineOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	if cfg.agentRuntime != nil {
		return &Container{
			FlowLoader:   flow.NewLoader(),
			AgentRuntime: cfg.agentRuntime,
		}, nil
	}

	if hub == nil {
		slog.Warn("engine: hub is nil; agent execution disabled")
		return &Container{
			FlowLoader:   flow.NewLoader(),
			AgentRuntime: nil,
		}, nil
	}

	runsBase := ""
	agentCfgDir := ""
	if cfg.homeDir != "" {
		runsBase = filepath.Join(cfg.homeDir, "runs")
		// Isolated Claude config dir for agent subprocesses. Keeping it under
		// the crowbar home directory ensures agents never read the host user's
		// ~/.claude (skills, settings, memory), which would make their
		// behaviour non-deterministic.
		agentCfgDir = filepath.Join(cfg.homeDir, "agent-claude")
	}

	runsDirFn := func(runID string) string { return runsDir(runsBase, runID) }

	return &Container{
		FlowLoader:   flow.NewLoader(),
		AgentRuntime: agent.New(config.GetIntelligence(), runsDirFn, hub, cfg.mcpURL, agentCfgDir),
	}, nil
}
