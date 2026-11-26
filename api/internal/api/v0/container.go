package v0

import (
	accesshandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/access/handlers"
	pairinghandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing/handlers"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/core/logring"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the v0 delivery surface: the REST routes and the stream route.
type Container struct {
	app *app.Container
	eng *engine.Container
	// access opens and closes device connections.
	access *accesshandlers.Handlers
	// pairing serves the pairing/client-registry routes (router.go); its
	// usecase holds the single outstanding pairing code in memory.
	pairing *pairinghandlers.Handlers
	// conns holds the streams each paired client opened, for revoke to end.
	conns *clientConns
	// logs backs GET /console/logs; nil leaves the route unmounted.
	logs logring.Ring
}

// Access is the device listener: its state, which a minted pairing code carries
// (the TLS identity the client checks it dialed), and the switch for it.
type Access interface {
	accesshandlers.Access
}

// New builds the v0 container.
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
