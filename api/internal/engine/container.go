package engine

import (
	"context"

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
// to manage them, and the PTY master FDs leak.
func (c *Container) Close() {
	if c.Terminal != nil {
		c.Terminal.Shutdown()
	}
}
