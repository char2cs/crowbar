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
		PRUrl:    "https://example.test/pr/open",
	}, now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{ID: "plain", RepoID: "r", ProjectID: "p"}, now)
	require.NoError(t, err)

	// Create/SyncProviderState commit via async Send, so the store projection that
	// List reads is eventually consistent (see rebuild_test.go: "SendWait makes the
	// store projection deterministic"). Poll until it converges — matching the
	// production sweep, which is itself a periodic poll.
	require.Eventually(t, func() bool {
		return len(sweepTargets(repo)()) == 1
	}, 5*time.Second, 20*time.Millisecond, "sweep must converge to exactly the open-PR workspace")
	targets := sweepTargets(repo)()
	require.Len(t, targets, 1)
	assert.Equal(t, "open", targets[0].WSID)
	assert.True(t, targets[0].HasOpenPR)
}

func TestSweeper_FiltersByPRUrlAndTerminalState(t *testing.T) {
	ctx := context.Background()
	c := newSweepContainer(t)
	repo := c.Repositories.Workspace
	now := time.Unix(1, 0).UTC()

	// open: live PR, non-terminal -> swept.
	mkProviderWS(t, repo, "open", "open", now)
	// merged: terminal -> never re-swept.
	mkProviderWS(t, repo, "merged", "merged", now)
	// closed: terminal -> never re-swept.
	mkProviderWS(t, repo, "closed", "closed", now)
	// conflicts: has a PRUrl but a local-conflict status; the widened filter
	// must still sweep it (the narrow Status==pr-open filter would drop it).
	mkProviderWS(t, repo, "conflicts", "open", now)
	_, err := repo.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           "conflicts",
		HasConflicts: true,
	}, now)
	require.NoError(t, err)
	conflicted, err := repo.Get(ctx, "conflicts")
	require.NoError(t, err)
	require.Equal(t, domain.WorkspaceStatusPRConflicts, conflicted.Status)
	require.NotEmpty(t, conflicted.PRUrl)
	// nopr: no PR at all -> never swept.
	_, err = repo.Create(ctx, workspace.CreateInput{ID: "nopr", RepoID: "r", ProjectID: "p"}, now)
	require.NoError(t, err)

	want := map[string]bool{
		"open":      true,
		"conflicts": true,
		"merged":    false,
		"closed":    false,
		"nopr":      false,
	}

	// Eventually-consistent read model (async Send projection): poll until the
	// sweep target set matches the expected inclusion map.
	require.Eventually(t, func() bool {
		got := make(map[string]bool)
		for _, tgt := range sweepTargets(repo)() {
			got[tgt.WSID] = true
		}
		for id, included := range want {
			if got[id] != included {
				return false
			}
		}
		return true
	}, 5*time.Second, 20*time.Millisecond, "sweep targets must converge to the expected set")

	// Every surfaced target must carry HasOpenPR.
	for _, tgt := range sweepTargets(repo)() {
		assert.Truef(t, tgt.HasOpenPR, "sweep target %s must carry HasOpenPR", tgt.WSID)
	}
}

func mkProviderWS(
	t *testing.T,
	repo workspace.Workspace,
	id string,
	prStatus string,
	now time.Time,
) {
	t.Helper()
	ctx := context.Background()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: id, RepoID: "r", ProjectID: "p"}, now)
	require.NoError(t, err)
	_, err = repo.SyncProviderState(ctx, workspace.ProviderInput{
		ID:       id,
		HasPR:    true,
		PRStatus: prStatus,
		PRUrl:    "https://example.test/pr/" + id,
	}, now)
	require.NoError(t, err)
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
