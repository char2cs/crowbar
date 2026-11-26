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

// serve runs a server's Serve, reporting its result on serveErr.
func (c *Container) serve(
	run func() error,
) {
	c.serving.Go(func() { c.serveErr <- run() })
}

// release undoes a New that fails: the gate stops serving, then the home is
// let go. The stores, if any were opened, are closed by then.
func (c *Container) release() error {
	c.exposure.Close()
	err := c.server.Close()
	c.serving.Wait()
	if c.session != nil {
		err = errors.Join(err, c.session.Close())
	}
	return err
}

// Run serves until ctx is cancelled, then shuts down gracefully. An early Serve failure is returned
// so the process exits non-zero and its supervisor restarts it, instead of hanging "up".
func (c *Container) Run(
	ctx context.Context,
) error {
	if c.stack != nil {
		c.serve(c.stack.hooks.Serve)
	}
	select {
	case <-ctx.Done():
		// Ordered shutdown: drain HTTP, then app.Shutdown quiesces writers before the deferred Close shuts
		// the DBs. The phases get separate deadlines so a slow request cannot starve the writer quiesce;
		// HTTP drain is short since the desktop supervisor SIGKILLs 3s after SIGTERM.
		const httpDrainGrace = 1 * time.Second
		httpCtx, cancelHTTP := context.WithTimeout(context.WithoutCancel(ctx), httpDrainGrace)
		defer cancelHTTP()
		c.exposure.Close()
		httpErr := c.server.Shutdown(httpCtx)
		if c.stack == nil {
			// A halted daemon opened no store, so it has no writer to drain.
			return httpErr
		}
		httpErr = errors.Join(httpErr, c.stack.hooks.Shutdown(httpCtx))

		drainCtx, cancelDrain := context.WithTimeout(context.WithoutCancel(ctx), app.WriterDrainGrace)
		defer cancelDrain()
		drainErr := c.stack.app.Shutdown(drainCtx)
		return errors.Join(httpErr, drainErr)
	case err := <-c.serveErr:
		// ErrServerClosed only occurs after Shutdown (handled above), so any error
		// arriving here is a genuine listen/accept failure.
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("internal: serve: %w", err)
		}
		return nil
	}
}

// Close stops serving and tears the backend down in dependency order (Run has
// usually shut the servers down already), then lets go of the home.
func (c *Container) Close() {
	c.exposure.Close()
	errs := []error{c.server.Close()}
	if c.stack != nil {
		errs = append(errs, c.stack.close())
	}
	c.serving.Wait()
	// Released last: the home is held until every store is closed.
	errs = append(errs, c.session.Close())
	if err := errors.Join(errs...); err != nil {
		slog.Warn("internal: close", "err", err)
	}
}
