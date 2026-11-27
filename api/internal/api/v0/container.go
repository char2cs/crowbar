package v0

import (
	accesshandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/access/handlers"
	pairinghandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing/handlers"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/core/logring"
	"github.com/char2cs/crowbar/api/internal/engine"
)

type Container struct {
	app     *app.Container
	eng     *engine.Container
	access  *accesshandlers.Handlers
	pairing *pairinghandlers.Handlers
	conns   *clientConns
	logs    logring.Ring
}

type Access interface {
	accesshandlers.Access
}

func New(
	appContainer *app.Container,
	engContainer *engine.Container,
	access Access,
	options ...Option,
) *Container {
	if appContainer == nil {
		panic("v0: appContainer is required")
	}
	conns := newClientConns()
	c := &Container{
		app:     appContainer,
		eng:     engContainer,
		pairing: pairinghandlers.New(pairing.New(appContainer.GORM.Clients), appContainer.GORM.Clients, conns, access),
		access:  accesshandlers.New(access),
		conns:   conns,
	}
	for _, option := range options {
		option(c)
	}
	if engContainer != nil && engContainer.Terminal != nil {
		if appContainer.Usecases != nil && appContainer.Usecases.TerminalMeta != nil {
			engContainer.Terminal.SetMetaStore(appContainer.Usecases.TerminalMeta)
		}
	}
	return c
}
