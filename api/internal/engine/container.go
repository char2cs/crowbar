package engine

import (
	"context"
	"sync/atomic"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	enginelsp "github.com/char2cs/crowbar/api/internal/engine/lsp"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
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

	// termDown latches the terminal engine's quiesce so it is entered exactly once.
	// See QuiesceTerminal: a second entrant must never BLOCK, because the first one
	// may be a caller that deliberately bounded its wait with a deadline.
	termDown atomic.Bool
}

type engineOpts struct {
	homeDir string
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

// New constructs the engine container.
func New(
	_ context.Context,
	opts ...Option,
) (*Container, error) {
	cfg := engineOpts{}
	for _, o := range opts {
		o(&cfg)
	}
	_ = cfg
	return &Container{
		Agents:   engineagents.New(),
		Git:      enginegit.New(),
		FS:       enginefs.New(),
		Provider: engineprovider.New(),
		Search:   enginesearch.New(),
		Terminal: engineterminal.New(),
		LSP:      enginelsp.New(nil),
		Identity: enginegit.NewIdentityEngine(),
	}, nil
}

// Close releases engine resources that own OS-level handles on daemon shutdown.
// Most engines are stateless facades, but the terminal engine holds live PTY
// child processes plus their master FDs; without this they are orphaned to init
// on shutdown/restart (dev hot-restart, crash-restart, OS quit) — the shell and
// any dev servers/builds it launched keep running and holding ports with no UI
// to manage them, and the PTY master FDs leak. The LSP host likewise holds live
// language-server subprocesses (gopls/tsserver/rust-analyzer) plus their stdio
// pipe FDs; without Shutdown every spawned server survives the daemon, leaking
// RAM, CPU, and FDs across restarts (R8).
func (c *Container) Close() {
	c.QuiesceTerminal()
	if c.LSP != nil {
		c.LSP.Shutdown(context.Background())
	}
}

// QuiesceTerminal kills every live PTY and BLOCKS until every exit callback those
// deaths fire has run to completion (engineterminal.Engine.Shutdown's contract).
//
// It is a step of the ORDERED graceful shutdown, not merely a resource release, and
// that is why it is exported: the app layer runs it FIRST — before it drains the
// aggregates and long before the adapter closes the databases — because those exit
// callbacks are WRITERS. A dying vendor CLI's callback is the only thing that records
// its death: it Exits the runner and closes the turn the CLI was mid-way through. Run
// it after the drain (which is where the mere resource-release reading of it lands, in
// Close) and those writes are rejected by a shut-down asynx or a closed DB — leaving a
// chat that spins forever, on every restart, with no live runner left for the next
// boot's reconcile to find. See app.Container.Shutdown.
//
// Entered exactly once. A second caller — the Close below, on the ordinary path where
// the app layer already quiesced — returns IMMEDIATELY rather than re-entering the
// join. That matters: the first caller may have bounded its wait with the shutdown
// deadline and moved on, and a re-entrant join would silently re-wedge the teardown on
// the very reaper the deadline was there to escape.
func (c *Container) QuiesceTerminal() {
	if c.Terminal == nil || !c.termDown.CompareAndSwap(false, true) {
		return
	}
	c.Terminal.Shutdown()
}
