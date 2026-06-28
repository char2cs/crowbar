package terminal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// fakeSessionStore is an in-memory store.Store[domain.TerminalSession, string].
type fakeSessionStore struct {
	rows    map[string]domain.TerminalSession
	saveErr error
	delErr  error
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{rows: make(map[string]domain.TerminalSession)}
}

func (s *fakeSessionStore) Save(
	_ context.Context,
	item domain.TerminalSession,
) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.rows[item.SessionID] = item
	return nil
}

func (s *fakeSessionStore) Delete(
	_ context.Context,
	id string,
) error {
	if s.delErr != nil {
		return s.delErr
	}
	delete(s.rows, id)
	return nil
}

func (s *fakeSessionStore) FindByKey(
	_ context.Context,
	id string,
) (*domain.TerminalSession, error) {
	row, ok := s.rows[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (s *fakeSessionStore) FindAll(
	_ context.Context,
) ([]domain.TerminalSession, error) {
	out := make([]domain.TerminalSession, 0, len(s.rows))
	for _, r := range s.rows {
		out = append(out, r)
	}
	return out, nil
}

// fakeWorkspaceRepo is a minimal WorkspaceRepo for testing.
type fakeWorkspaceRepo struct {
	ws  domain.Workspace
	err error
}

func (r *fakeWorkspaceRepo) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return r.ws, r.err
}

func buildMetaStore(
	t *testing.T,
	repo terminal.WorkspaceRepo,
	store *fakeSessionStore,
	home string,
) engineterminal.SessionMetaStore {
	t.Helper()
	return terminal.NewSessionMetaStore(
		repo,
		store,
		func() (string, error) { return home, nil },
	)
}

func TestSessionMetaStore_Save_UpsertsSetsProjectAndRepo(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWorkspaceRepo{
		ws: domain.Workspace{
			ID:        "ws-1",
			ProjectID: "proj-1",
			RepoID:    "repo-1",
		},
	}
	store := newFakeSessionStore()
	ms := buildMetaStore(t, repo, store, "/home")

	meta := engineterminal.SessionMeta{
		SessionID:    "sess-1",
		WorkspaceID:  "ws-1",
		CWD:          "/tmp",
		Shell:        "/bin/zsh",
		ProfileID:    "prof-1",
		State:        "active",
		LastActiveAt: time.Now().UTC(),
	}

	require.NoError(t, ms.Save(ctx, meta))

	row, ok := store.rows["sess-1"]
	require.True(t, ok)
	assert.Equal(t, "proj-1", row.ProjectID)
	assert.Equal(t, "repo-1", row.RepoID)
	assert.Equal(t, "/tmp", row.CWD)
	assert.Equal(t, "active", row.State)
}

func TestSessionMetaStore_Save_PreservesCreatedAtOnUpdate(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWorkspaceRepo{ws: domain.Workspace{ID: "ws-1", ProjectID: "p", RepoID: "r"}}
	store := newFakeSessionStore()
	ms := buildMetaStore(t, repo, store, "/home")

	original := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store.rows["sess-1"] = domain.TerminalSession{
		SessionID: "sess-1",
		CreatedAt: original,
	}

	meta := engineterminal.SessionMeta{
		SessionID:   "sess-1",
		WorkspaceID: "ws-1",
		State:       "detached",
	}
	require.NoError(t, ms.Save(ctx, meta))

	row := store.rows["sess-1"]
	assert.Equal(t, original, row.CreatedAt, "CreatedAt must be preserved on update")
}

func TestSessionMetaStore_Save_WorkspaceError(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWorkspaceRepo{err: errors.New("boom")}
	store := newFakeSessionStore()
	ms := buildMetaStore(t, repo, store, "/home")

	err := ms.Save(ctx, engineterminal.SessionMeta{SessionID: "s", WorkspaceID: "ws"})
	assert.Error(t, err)
}

func TestSessionMetaStore_Delete_RemovesRow(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWorkspaceRepo{ws: domain.Workspace{ID: "ws-1"}}
	store := newFakeSessionStore()
	store.rows["sess-1"] = domain.TerminalSession{SessionID: "sess-1"}
	ms := buildMetaStore(t, repo, store, "/home")

	require.NoError(t, ms.Delete(ctx, "sess-1"))
	_, ok := store.rows["sess-1"]
	assert.False(t, ok)
}

func TestSessionMetaStore_Delete_MissingOK(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWorkspaceRepo{}
	store := newFakeSessionStore()
	ms := buildMetaStore(t, repo, store, "/home")

	// Delete of non-existent session should not error.
	assert.NoError(t, ms.Delete(ctx, "no-such-session"))
}

func TestSessionMetaStore_StorageDir_ResolvesPath(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWorkspaceRepo{
		ws: domain.Workspace{
			ID:        "ws-1",
			ProjectID: "proj-1",
			RepoID:    "repo-1",
		},
	}
	store := newFakeSessionStore()
	ms := buildMetaStore(t, repo, store, "/home/.crowbar")

	dir, err := ms.StorageDir(ctx, "ws-1")
	require.NoError(t, err)
	// Expected: /home/.crowbar/projects/proj-1/repo-1/workspaces/ws-1/storages
	assert.Contains(t, dir, "proj-1")
	assert.Contains(t, dir, "repo-1")
	assert.Contains(t, dir, "ws-1")
	assert.Contains(t, dir, "storages")
}

func TestSessionMetaStore_StorageDir_WorkspaceError(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWorkspaceRepo{err: errors.New("not found")}
	store := newFakeSessionStore()
	ms := buildMetaStore(t, repo, store, "/home")

	_, err := ms.StorageDir(ctx, "ws-1")
	assert.Error(t, err)
}
