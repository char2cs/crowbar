package v0

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

var errSnapshotFake = errors.New("snapshot fake")

type errWorkspaceRepo struct {
	workspace.Workspace
}

func (errWorkspaceRepo) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return nil, errSnapshotFake
}

func newAppForSnapshot(
	t *testing.T,
) *app.Container {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	return a
}

func TestWorkspacesSnapshot_ListErrorReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	a.Repositories.Workspace = errWorkspaceRepo{}
	assert.Nil(t, workspacesSnapshot(a)(""))
}

func TestGitSnapshot_ListErrorReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	a.Repositories.Workspace = errWorkspaceRepo{}
	assert.Nil(t, gitSnapshot(a)(""))
}

func TestGitSnapshot_BadWorktreeSkipsWorkspace(t *testing.T) {
	a := newAppForSnapshot(t)
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", WorktreePath: t.TempDir()},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
	assert.Empty(t, gitSnapshot(a)(""))
}

func TestLSPSnapshot_NilEngineReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	assert.Nil(t, lspSnapshot(a, nil))
	assert.Nil(t, lspSnapshot(a, &engine.Container{}))
}

func TestLSPSnapshot_ListErrorReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	a.Repositories.Workspace = errWorkspaceRepo{}
	eng, err := engine.New(context.Background())
	require.NoError(t, err)
	assert.Nil(t, lspSnapshot(a, eng)(""))
}

func TestLSPSnapshot_NoDiagnosticsIsEmpty(t *testing.T) {
	a := newAppForSnapshot(t)
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
	eng, err := engine.New(context.Background())
	require.NoError(t, err)
	assert.Empty(t, lspSnapshot(a, eng)(""))
}
