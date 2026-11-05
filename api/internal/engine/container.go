package engine

import (
	"context"
	"sync/atomic"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	enginelsp "github.com/char2cs/crowbar/api/internal/engine/lsp"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

// Container holds engine-layer dependencies. The AI Bridge engine and addon
// registry are added in later waves.
type Container struct {
	// Agents maps provider-owned CLI facts into Crowbar-neutral values: spawn
	// plans, hook and telemetry interpretation, and bounded capability probes.
	Agents   engineagents.Agents
	Git      enginegit.Engine
	FS       enginefs.Engine
	Provider engineprovider.Engine
	Search   enginesearch.SearchEngine
	Terminal engineterminal.Engine
	// LSP is the LSP host facade. It is always non-nil; graceful absence (no
	// server installed for a language) is signalled by empty results rather than
	// errors (10 §5).
	LSP      enginelsp.Engine
	Identity *enginegit.IdentityEngine
	// DaemonAddress is the address the daemon bound (unix:// or tcp://). Callbacks
	// are handed it, never one derived from the home (see WithDaemonAddress).
	DaemonAddress string
	// CrowbarBin is the crowbar binary a spawned CLI runs; empty is the one
	// installed under the home. A daemon run from a build that is not installed
	// there sets it before the app layer is built.
	CrowbarBin string
	// NoAPITransport runs every provider over hooks alone: no spawn forks a
	// provider's serve process.
	NoAPITransport bool

	// termDown latches the terminal engine's quiesce so it is entered exactly once.
	// See QuiesceTerminal: a second entrant must never BLOCK, because the first one
	// may be a caller that deliberately bounded its wait with a deadline.
	termDown atomic.Bool
}

type engineOpts struct {
	homeDir       string
	daemonAddress string
}

// Option configures engine.New.
type Option func(*engineOpts)

// WithHomeDir overrides the home directory used for path resolution.
func WithHomeDir(
	dir string,
) Option {
	return func(o *engineOpts) {
		o.homeDir = dir
	}
}

// WithDaemonAddress records the address the daemon bound, for the engines that
// hand callbacks back into it.
func WithDaemonAddress(
	address string,
) Option {
	return func(o *engineOpts) {
		o.daemonAddress = address
	}
}

// New constructs the engine container. ctx is the daemon's lifecycle context, passed to
// Agents so model-discovery refreshes stop on shutdown.
func New(
	ctx context.Context,
	opts ...Option,
) (*Container, error) {
	cfg := engineOpts{}
	for _, o := range opts {
		o(&cfg)
	}
	return &Container{
		DaemonAddress: cfg.daemonAddress,
		Agents: engineagents.New(engineagents.WithLifecycle(ctx),
			engineagents.WithOverrideCheck(descriptorcheck.AcceptOverride)),
		Git:      enginegit.New(),
		FS:       enginefs.New(),
		Provider: engineprovider.New(),
		Search:   enginesearch.New(),
		Terminal: engineterminal.New(),
		LSP:      enginelsp.New(nil),
		Identity: enginegit.NewIdentityEngine(),
	}, nil
}

// Close releases engine resources that own OS handles: the terminal's PTY children and
// FDs, the LSP host's server subprocesses, and the agents engine's discovery refreshes,
// which it joins so no write lands in a home this process has released.
func (c *Container) Close() {
	c.QuiesceTerminal()
	if c.LSP != nil {
		c.LSP.Shutdown(context.Background())
	}
	if c.Agents != nil {
		c.Agents.Close()
	}
}

// QuiesceTerminal kills every live PTY and blocks until their exit callbacks finish.
// It is exported because Shutdown must run it before draining aggregates and closing
// DBs, which those callbacks write into; a second call returns immediately, never re-joining.
func (c *Container) QuiesceTerminal() {
	if c.Terminal == nil || !c.termDown.CompareAndSwap(false, true) {
		return
	}
	c.Terminal.Shutdown()
}
