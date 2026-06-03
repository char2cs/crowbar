package internal

import (
	"context"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/core/gateway"
	"github.com/char2cs/crowbar/api/internal/engine"
)

type Container struct {
	server   *http.Server
	listener net.Listener
	adapter  *adapter.Container
}

func New(ctx context.Context, host string, staticFS fs.FS) (*Container, error) {
	listener, err := gateway.New(host)
	if err != nil {
		return nil, err
	}

	engines, err := engine.New(ctx)
	if err != nil {
		return nil, err
	}

	adapters, err := adapter.New()
	if err != nil {
		return nil, err
	}

	appContainer, err := app.New()
	if err != nil {
		return nil, err
	}

	go appContainer.Hub.Run(ctx)

	apiContainer, err := crowbarapi.New(appContainer, staticFS)
	if err != nil {
		return nil, err
	}

	_ = engines // engines unused at scaffold stage; used in future phases

	srv := &http.Server{Handler: apiContainer.Handler()}

	return &Container{server: srv, listener: listener, adapter: adapters}, nil
}

func (c *Container) Run(ctx context.Context) error {
	go c.server.Serve(c.listener) //nolint:errcheck
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.server.Shutdown(shutdownCtx)
}

func (c *Container) Close() {
	c.adapter.Close()
}
