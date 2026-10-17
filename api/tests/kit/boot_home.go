//go:build integration

package kit

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

// BootHome does what the daemon does to dir before it opens any store: it
// takes the home and runs the pre-open migration steps. Pass the session to
// app.New and close it after the stores. A halted boot returns the session
// with its *migrate.Halt; any other failure returns no session.
func BootHome(
	ctx context.Context,
	dir string,
) (*migrate.Session, error) {
	h, err := home.At(dir)
	if err != nil {
		return nil, fmt.Errorf("kit: home: %w", err)
	}
	session, err := migrations.Begin(ctx, h, migrate.Options{Version: metadata.GetVersion()})
	if err != nil {
		return nil, fmt.Errorf("kit: migrate: %w", err)
	}
	err = session.RunPreOpen(ctx, migrations.PreOpenImpls())
	var halt *migrate.Halt
	if err == nil || errors.As(err, &halt) {
		return session, err
	}
	return nil, errors.Join(fmt.Errorf("kit: migrate: %w", err), session.Close())
}

// releaseHome adds releasing session, when there is one, to a boot that failed with err.
func releaseHome(
	session *migrate.Session,
	err error,
) error {
	if session == nil {
		return err
	}
	return errors.Join(err, session.Close())
}

// releaseStores closes the env's stores, then the home they were opened in.
func (e *Env) releaseStores(
	t *testing.T,
	teardown string,
) {
	t.Helper()
	if err := e.adapters.Close(); err != nil {
		t.Logf("kit.Env.%s: adapter: %v", teardown, err)
	}
	if err := e.session.Close(); err != nil {
		t.Logf("kit.Env.%s: home: %v", teardown, err)
	}
}
