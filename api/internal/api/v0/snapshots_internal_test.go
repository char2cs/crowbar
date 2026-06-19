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

// TestWorkspacesSnapshot_ScopedToRepo proves the snapshot is filtered to the
// repo parsed from the client's subscription prefix ("p/r"): only that repo's
// workspaces are returned, as wire DTOs (spec §9).
func TestWorkspacesSnapshot_ScopedToRepo(t *testing.T) {
	a := newAppForSnapshot(t)
	ctx := context.Background()
	seedWorkspace(t, a, "w1", "p1", "r1", "", "")
	seedWorkspace(t, a, "w2", "p1", "r2", "", "")
	seedWorkspace(t, a, "w3", "p2", "r1", "", "")
	require.NoError(t, ctx.Err())

	got := workspacesSnapshot(a)("p1/r1")
	require.Len(t, got, 1)
	assert.Equal(t, "w1", got[0].ID)
	assert.Equal(t, "p1", got[0].ProjectID)
	assert.Equal(t, "r1", got[0].RepoID)
}

// TestWorkspacesSnapshot_ComputesCanMergeLocally proves the snapshot DTOs carry
// the merge-eligibility overlay resolved from repo siblings: a child whose
// parent is a same-repo non-locked sibling is eligible with the parent's branch
// (spec §10).
func TestWorkspacesSnapshot_ComputesCanMergeLocally(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "parent", "p1", "r1", "main", "")
	seedWorkspace(t, a, "child", "p1", "r1", "feat", "parent")

	got := workspacesSnapshot(a)("p1/r1")
	require.Len(t, got, 2)

	byID := map[string]bool{}
	branch := map[string]string{}
	for _, d := range got {
		byID[d.ID] = d.CanMergeLocally
		branch[d.ID] = d.ParentBranch
	}
	assert.True(t, byID["child"])
	assert.Equal(t, "main", branch["child"])
	assert.False(t, byID["parent"])
	assert.Equal(t, "", branch["parent"])
}

func seedWorkspace(
	t *testing.T,
	a *app.Container,
	id string,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) {
	t.Helper()
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{
			ID:        id,
			ProjectID: projectID,
			RepoID:    repoID,
			Branch:    branch,
			ParentID:  parentID,
		},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
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
