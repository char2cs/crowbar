package v0

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
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

// errProjectStore is a project store whose FindAll always fails, exercising the
// snapshot's degrade-to-nil path.
type errProjectStore struct {
	store.Store[domain.Project, string]
}

func (errProjectStore) FindAll(
	_ context.Context,
) ([]domain.Project, error) {
	return nil, errSnapshotFake
}

// errRepoStore is a repository store whose FindAll always fails, exercising the
// snapshot's degrade-to-nil path.
type errRepoStore struct {
	store.ScopedStore[domain.Repository, string]
}

func (errRepoStore) FindAll(
	_ context.Context,
) ([]domain.Repository, error) {
	return nil, errSnapshotFake
}

// errReviewThreadRepo is a ReviewThread repo whose ListByWorkspace always
// fails, exercising threadsSnapshot's degrade-to-nil path.
type errReviewThreadRepo struct {
	reviewthread.ReviewThread
}

func (errReviewThreadRepo) ListByWorkspace(
	_ context.Context,
	_ string,
) ([]domain.ReviewThread, error) {
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
	// The workspace store projection is async (Send, not SendWait): drain every
	// per-type asynx instance so the new row is visible in the read model, then
	// assert its presence synchronously before the snapshot reads it.
	a.Repositories.WaitQuiescent()
	rows, err := a.Repositories.Workspace.List(context.Background())
	require.NoError(t, err)
	found := false
	for _, r := range rows {
		if r.ID == id {
			found = true
			break
		}
	}
	require.True(t, found, "seeded workspace %q must be visible in the read model after drain", id)
}

// TestProjectSnapshot proves the Projects snapshot-on-subscribe (03 §1a)
// returns every project row as wire DTOs. Projects sit at the top of the
// hierarchy, so a list-level (empty) or project-level scope both snapshot the
// full project set; the per-client predicate then filters by prefix.
func TestProjectSnapshot(t *testing.T) {
	a := newAppForSnapshot(t)
	ctx := context.Background()
	require.NoError(t, a.GORM.Projects.Save(ctx, domain.Project{ID: "p1", Name: "Alpha"}))
	require.NoError(t, a.GORM.Projects.Save(ctx, domain.Project{ID: "p2", Name: "Beta"}))

	got := projectSnapshot(a)("")
	require.Len(t, got, 2)

	byID := map[string]string{}
	for _, d := range got {
		byID[d.ID] = d.Name
	}
	assert.Equal(t, "Alpha", byID["p1"])
	assert.Equal(t, "Beta", byID["p2"])
}

// TestProjectSnapshot_ListErrorReturnsNil proves a failed project list degrades
// to a nil snapshot rather than panicking.
func TestProjectSnapshot_ListErrorReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	a.GORM.Projects = errProjectStore{}
	assert.Nil(t, projectSnapshot(a)(""))
}

// TestRepoSnapshot proves the Repos snapshot-on-subscribe (03 §1a) returns the
// repos under the project parsed from the client's subscription prefix ("p/..."),
// as wire DTOs. Repos under a sibling project are excluded.
func TestRepoSnapshot(t *testing.T) {
	a := newAppForSnapshot(t)
	ctx := context.Background()
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Name: "one"}))
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1", Name: "two"}))
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r3", ProjectID: "p2", Name: "three"}))

	got := repoSnapshot(a)("p1")
	require.Len(t, got, 2)

	byID := map[string]string{}
	for _, d := range got {
		byID[d.ID] = d.ProjectID
	}
	assert.Equal(t, "p1", byID["r1"])
	assert.Equal(t, "p1", byID["r2"])
	_, hasR3 := byID["r3"]
	assert.False(t, hasR3, "sibling-project repo must be excluded")
}

// TestRepoSnapshot_EmptyScopeReturnsAll proves a list-level (empty) scope keeps
// every repo: an empty project component matches all projects.
func TestRepoSnapshot_EmptyScopeReturnsAll(t *testing.T) {
	a := newAppForSnapshot(t)
	ctx := context.Background()
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r3", ProjectID: "p2"}))

	got := repoSnapshot(a)("")
	assert.Len(t, got, 2)
}

// TestRepoSnapshot_ListErrorReturnsNil proves a failed repo list degrades to a
// nil snapshot.
func TestRepoSnapshot_ListErrorReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	a.GORM.Repositories = errRepoStore{}
	assert.Nil(t, repoSnapshot(a)("p1"))
}

// TestThreadsSnapshot_NoWorkspaceSegmentReturnsNil covers the guard at the top
// of threadsSnapshot: threads are always workspace-scoped, so a repo- or
// project-level subscription (fewer than 3 scope segments, or an empty
// workspace segment) must yield nil rather than attempting a global
// enumeration of the ReviewThread aggregate.
func TestThreadsSnapshot_NoWorkspaceSegmentReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	snap := threadsSnapshot(a)
	require.NotNil(t, snap)

	assert.Nil(t, snap(""))
	assert.Nil(t, snap("p1"))
	assert.Nil(t, snap("p1/r1"))
	assert.Nil(t, snap("p1/r1/"))
}

// TestThreadsSnapshot_ListErrorReturnsNil proves a failed ListByWorkspace
// degrades to a nil snapshot rather than failing the subscribe.
func TestThreadsSnapshot_ListErrorReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	a.Repositories.ReviewThread = errReviewThreadRepo{}

	assert.Nil(t, threadsSnapshot(a)("p1/r1/w1"))
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

// TestLSPSnapshot_UnknownWorkspaceScope_ReturnsNil covers the guard that
// preserves scopedWorkspaceRows' literal nil (an unresolvable workspace id)
// rather than upgrading it to a non-nil empty slice — mirroring
// TestGitSnapshot_UnknownWorkspaceScope_ReturnsNil for the LSP source.
func TestLSPSnapshot_UnknownWorkspaceScope_ReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	eng, err := engine.New(context.Background())
	require.NoError(t, err)

	assert.Nil(t, lspSnapshot(a, eng)("does-not-exist"))
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

// TestGitSnapshot_ScopedToWorkspaceRepo proves gitSnapshot resolves the
// subscribing workspace id to its repo and only touches that repo's
// workspaces — not every workspace in the install.
func TestGitSnapshot_ScopedToWorkspaceRepo(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "w1", "p1", "r1", "", "")
	seedWorkspace(t, a, "w2", "p2", "r2", "", "")

	got := gitSnapshot(a)("p1/r1/w1")

	ids := make([]string, len(got))
	for i, e := range got {
		ids[i] = e.WsID
	}
	assert.Contains(t, ids, "w1")
	assert.NotContains(t, ids, "w2")
}

// TestGitSnapshot_ExcludesSameRepoSibling proves gitSnapshot no longer computes
// git status for the resolved workspace's repo siblings — only the resolved
// workspace itself — since the broadcaster's wsId predicate discards every
// other row after delivery anyway (03 §1a / container.go ScopeKey). Before this
// fix, a same-repo sibling (w3, same p1/r1 as w1) WOULD have appeared in the
// snapshot returned by scopedWorkspaceRows (only a different-repo workspace was
// excluded); this test proves it's excluded even at the snapshot-builder level
// now, not just downstream by the broadcaster's predicate.
func TestGitSnapshot_ExcludesSameRepoSibling(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "w1", "p1", "r1", "", "")
	seedWorkspace(t, a, "w3", "p1", "r1", "", "")

	got := gitSnapshot(a)("p1/r1/w1")

	ids := make([]string, len(got))
	for i, e := range got {
		ids[i] = e.WsID
	}
	assert.Equal(t, []string{"w1"}, ids)
}

func TestLSPSnapshot_ScopedToWorkspaceRepo(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "w1", "p1", "r1", "", "")
	seedWorkspace(t, a, "w2", "p2", "r2", "", "")
	eng, err := engine.New(context.Background())
	require.NoError(t, err)

	assert.NotPanics(t, func() { lspSnapshot(a, eng)("p1/r1/w1") })
}

// TestLSPSnapshot_UnknownChatScope_ReturnsNil covers the OTHER shape a scope
// arrives in now: a bare id from the chat-scoped route. A chat id nothing
// resolves to degrades to an empty replay, exactly as an unknown workspace
// does — the subscription still opens, it simply has nothing to replay.
func TestLSPSnapshot_UnknownChatScope_ReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	eng, err := engine.New(context.Background())
	require.NoError(t, err)

	assert.Nil(t, lspSnapshot(a, eng)("chat-does-not-exist"))
}

// TestLSPSnapshot_BareScopeIsNotReadAsAWorkspaceID pins the meaning change a
// silent mis-read would otherwise hide, mirroring gitSnapshot's own guard.
//
// A bare scope reaches lspSnapshot only from /v0/chats/:chatId/lsp/ws, so it
// is a CHAT id and has to be resolved via worktree.Resolve — it must NOT be
// taken verbatim as a workspace id, or a real workspace id passed bare would
// wrongly succeed.
func TestLSPSnapshot_BareScopeIsNotReadAsAWorkspaceID(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "w1", "p1", "r1", "", "")
	eng, err := engine.New(context.Background())
	require.NoError(t, err)

	assert.Empty(t, lspSnapshot(a, eng)("w1"),
		"a bare id is a chat id: no chat is called w1, so there is nothing to replay")
}

func TestGitSnapshot_UnknownWorkspaceScope_ReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	assert.Nil(t, gitSnapshot(a)("p1/r1/does-not-exist"))
}

// TestGitSnapshot_UnknownChatScope_ReturnsNil covers the OTHER shape a scope
// arrives in now: a bare id from the chat-scoped route. An id no chat answers
// to degrades to an empty replay, exactly as an unknown workspace does — the
// subscription still opens, it simply has nothing to replay.
func TestGitSnapshot_UnknownChatScope_ReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	assert.Nil(t, gitSnapshot(a)("chat-does-not-exist"))
}

// TestGitSnapshot_BareScopeIsNotReadAsAWorkspaceID pins the meaning change a
// silent mis-read would otherwise hide.
//
// A bare scope reaches gitSnapshot only from /v0/chats/:chatId/git/status, so
// it is a CHAT id and has to be resolved. It used to be taken verbatim as a
// workspace id — and if that reading survived, a chat id would simply fail to
// match any workspace and the snapshot would look correctly empty, while a
// WORKSPACE id passed bare would wrongly succeed. That second half is what this
// asserts: w1 exists, and naming it bare must NOT replay it.
func TestGitSnapshot_BareScopeIsNotReadAsAWorkspaceID(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "w1", "p1", "r1", "", "")

	require.NotEmpty(t, gitSnapshot(a)("p1/r1/w1"),
		"the hierarchical scope must still replay the workspace it names")
	assert.Empty(t, gitSnapshot(a)("w1"),
		"a bare id is a chat id: no chat is called w1, so there is nothing to replay")
}
