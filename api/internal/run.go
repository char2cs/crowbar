package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/char2cs/crowbar/api/internal/app"
)

func (c *Container) serve(
	run func() error,
) {
	c.serving.Go(func() { c.serveErr <- run() })
}

func (c *Container) release() error {
	c.exposure.Close()
	err := c.server.Close()
	c.serving.Wait()
	if c.session != nil {
		err = errors.Join(err, c.session.Close())
	}
	return err
}

func (c *Container) Run(
	ctx context.Context,
) error {
	if c.stack != nil {
		c.serve(c.stack.hooks.Serve)
	}
	select {
	case <-ctx.Done():
		const httpDrainGrace = 1 * time.Second
		httpCtx, cancelHTTP := context.WithTimeout(context.WithoutCancel(ctx), httpDrainGrace)
		defer cancelHTTP()
		c.exposure.Close()
		httpErr := c.server.Shutdown(httpCtx)
		if c.stack == nil {
			return httpErr
		}
		httpErr = errors.Join(httpErr, c.stack.hooks.Shutdown(httpCtx))

		drainCtx, cancelDrain := context.WithTimeout(context.WithoutCancel(ctx), app.WriterDrainGrace)
		defer cancelDrain()
		drainErr := c.stack.app.Shutdown(drainCtx)
		return errors.Join(httpErr, drainErr)
	case err := <-c.serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("internal: serve: %w", err)
		}
		return nil
	}
}

func (c *Container) Close() {
	c.exposure.Close()
	errs := []error{c.server.Close()}
	if c.stack != nil {
		errs = append(errs, c.stack.close())
	}
	c.serving.Wait()
	errs = append(errs, c.session.Close())
	if err := errors.Join(errs...); err != nil {
		slog.Warn("internal: close", "err", err)
	}
}
