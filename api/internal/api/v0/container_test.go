//go:build integration

package v0_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

type testContainers struct {
	app *app.Container
	eng *engine.Container
}

func newApp(t *testing.T) testContainers {
	t.Helper()
	ctx := t.Context()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	t.Cleanup(eng.Close)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters, migratetest.NoPostOpen(), v0.EncodeEntity, v0.NewSidebar)
	require.NoError(t, err)
	// app.Close stops the asynx pools; t.Context's cancel stops the sweeps.
	t.Cleanup(a.Close)
	return testContainers{app: a, eng: eng}
}

// seedRepo creates a real repository row under project p1 so the repo scope guard
// (scopeRepoToPath) admits a request to /projects/p1/repos/:repoId/...; a :repoId
// that resolves to no row, or to a different project, is now 404'd before the
// handler. Save is an upsert, so seeding the same repo twice is harmless.
func seedRepoIn(t *testing.T, tc testContainers, projectID, repoID string) {
	t.Helper()
	require.NoError(t, tc.app.GORM.Repositories.Save(
		context.Background(),
		domain.Repository{ID: repoID, ProjectID: projectID, Name: repoID, Path: t.TempDir()},
	))
}

func seedRepo(t *testing.T, tc testContainers, repoID string) {
	t.Helper()
	seedRepoIn(t, tc, "p1", repoID)
}

// seedWorkspace creates a real workspace row under p1/r1 (plus its repo) so both
// scope guards (scopeRepoToPath, scopeWorkspaceToPath) admit a request/WS upgrade
// to /workspaces/:id/...; a :repoId/:wsId that resolves to no row, or to a
// different scope, is now 404'd before the handler.
func seedWorkspace(t *testing.T, tc testContainers, id string) {
	t.Helper()
	seedRepo(t, tc, "r1")
	_, err := tc.app.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: id, RepoID: "r1", ProjectID: "p1", WorktreePath: t.TempDir(), Provisioning: domain.WorkspaceProvisioned},
		time.Now(),
	)
	require.NoError(t, err)
}
