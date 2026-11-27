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
	t.Cleanup(a.Close)
	return testContainers{app: a, eng: eng}
}

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
