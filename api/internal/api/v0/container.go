package v0

import (
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/core/logring"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the v0 delivery surface: the REST routes and the stream route.
type Container struct {
	app *app.Container
	eng *engine.Container
	// logs backs GET /console/logs; nil leaves the route unmounted.
	logs logring.Ring
}

// New builds the v0 container.
func New(
	appContainer *app.Container,
	engContainer *engine.Container,
	options ...Option,
) *Container {
	if appContainer == nil {
		panic("v0: appContainer is required")
	}
	c := &Container{
		app: appContainer,
		eng: engContainer,
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
