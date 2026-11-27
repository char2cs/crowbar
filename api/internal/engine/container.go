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

type Container struct {
	Agents         engineagents.Agents
	Git            enginegit.Engine
	FS             enginefs.Engine
	Provider       engineprovider.Engine
	Search         enginesearch.SearchEngine
	Terminal       engineterminal.Engine
	LSP            enginelsp.Engine
	Identity       *enginegit.IdentityEngine
	DaemonAddress  string
	CrowbarBin     string
	NoAPITransport bool

	termDown atomic.Bool
}

type engineOpts struct {
	homeDir       string
	daemonAddress string
}

type Option func(*engineOpts)

func WithHomeDir(
	dir string,
) Option {
	return func(o *engineOpts) {
		o.homeDir = dir
	}
}

func WithDaemonAddress(
	address string,
) Option {
	return func(o *engineOpts) {
		o.daemonAddress = address
	}
}

func New(
	ctx context.Context,
	opts ...Option,
) (*Container, error) {
	cfg := engineOpts{}
	for _, o := range opts {
		o(&cfg)
	}
	self, err := selfExecutable()
	if err != nil {
		return nil, err
	}
	return &Container{
		DaemonAddress: cfg.daemonAddress,
		CrowbarBin:    self,
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

func (c *Container) Close() {
	c.QuiesceTerminal()
	if c.LSP != nil {
		c.LSP.Shutdown(context.Background())
	}
	if c.Agents != nil {
		c.Agents.Close()
	}
}

func (c *Container) QuiesceTerminal() {
	if c.Terminal == nil || !c.termDown.CompareAndSwap(false, true) {
		return
	}
	c.Terminal.Shutdown()
}
