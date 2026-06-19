package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/engine/provider"
)

func newSweepContainer(
	t *testing.T,
) *Container {
	t.Helper()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	eng, err := engine.New(context.Background())
	require.NoError(t, err)
	c, err := New(context.Background(), eng, adapters)
	require.NoError(t, err)
	return c
}

func TestSweepTargets_FiltersOpenPROnly(t *testing.T) {
	ctx := context.Background()
	c := newSweepContainer(t)
	repo := c.Repositories.Workspace
	now := time.Unix(1, 0).UTC()

	_, err := repo.Create(ctx, workspace.CreateInput{ID: "open", RepoID: "r", ProjectID: "p"}, now)
	require.NoError(t, err)
	_, err = repo.SyncProviderState(ctx, workspace.ProviderInput{
		ID:       "open",
		HasPR:    true,
		PRStatus: "open",
	}, now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{ID: "plain", RepoID: "r", ProjectID: "p"}, now)
	require.NoError(t, err)

	targets := sweepTargets(repo)()
	require.Len(t, targets, 1)
	assert.Equal(t, "open", targets[0].WSID)
	assert.True(t, targets[0].HasOpenPR)
}

type listErrWorkspace struct {
	workspace.Workspace
}

func (listErrWorkspace) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return nil, errors.New("boom")
}

func TestSweepTargets_ListError_ReturnsNil(t *testing.T) {
	targets := sweepTargets(listErrWorkspace{})()
	assert.Nil(t, targets)
}

func TestSweepCallback_AppliesProviderState(t *testing.T) {
	ctx := context.Background()
	c := newSweepContainer(t)
	repo := c.Repositories.Workspace
	now := time.Unix(1, 0).UTC()

	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r", ProjectID: "p"}, now)
	require.NoError(t, err)

	cb := sweepCallback(ctx, c.Usecases)
	cb("w1", provider.ProviderState{Protected: true})

	got, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusLocked, got.Status)
}
