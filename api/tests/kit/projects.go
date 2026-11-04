//go:build integration

package kit

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// RegisterProject creates a project via POST /v0/projects and returns its id from the projects
// scope, held BEFORE the POST so the created row is never missed.
func (e *Env) RegisterProject(
	t *testing.T,
	name string,
	path string,
) string {
	t.Helper()
	w := e.WatchProjects(t)
	resp := e.POST(t, "/v0/projects", map[string]any{
		"name": name,
		"path": path,
	})
	RequireStatus(t, resp, http.StatusAccepted)
	require.NoError(t, resp.Body.Close())
	msg := w.Next(t, 10*time.Second, stream.KindProject, func(m map[string]any) bool {
		p, _ := m["path"].(string)
		return p == path
	})
	id, _ := msg["id"].(string)
	require.NotEmpty(t, id, "RegisterProject: ProjectDTO must carry an id")
	return id
}

// RegisterRepo creates a repo and returns its id from the projects scope, held BEFORE the POST
// so the row is never missed. path may be empty for repos that need no git operations.
func (e *Env) RegisterRepo(
	t *testing.T,
	projectID string,
	name string,
	path string,
) string {
	t.Helper()
	w := e.WatchProjects(t)
	resp := e.POST(t, "/v0/projects/"+projectID+"/repos", map[string]any{
		"name": name,
		"path": path,
	})
	RequireStatus(t, resp, http.StatusAccepted)
	require.NoError(t, resp.Body.Close())
	msg := w.Next(t, 10*time.Second, stream.KindRepo, func(m map[string]any) bool {
		return m["name"] == name && m["projectId"] == projectID
	})
	id, _ := msg["id"].(string)
	require.NotEmpty(t, id, "RegisterRepo: RepoDTO must carry an id")
	return id
}

// ImportedRepo bundles a full project+repo import's ids: project, repo, the adopted main
// workspace, and ChatID, its owning chat, which the import already minted.
type ImportedRepo struct {
	ProjectID   string
	RepoID      string
	WorkspaceID string
	ChatID      string
	RepoPath    string
}

// ImportRepo creates (or inits) a git repo, imports it as a project and runs the full repo import,
// returning the ids and path; the adopted workspace comes from the WS broadcast, dialled first.
func (e *Env) ImportRepo(
	t *testing.T,
	name string,
	path string,
) ImportedRepo {
	t.Helper()
	if path == "" {
		path = InitRepo(t)
	}
	// Cleanups run last-registered first: without this the repo's TempDir would
	// be removed while the daemon still works in it.
	t.Cleanup(func() { e.Close(t) })
	projectID := e.RegisterProject(t, name, path)
	// The importer names the repo from its directory, so match the RepoDTO by projectId; hold
	// projects BEFORE the POST so the row is never missed.
	reposWS := e.WatchProjects(t)
	resp := e.POST(t, "/v0/projects/"+projectID+"/repos", map[string]any{ //nolint:bodyclose // AwaitOperation closes the body
		"name": name,
		"path": path,
	})
	// The repo row lands as the import starts; its worktrees by the time the
	// import's operation settles.
	e.AwaitOperation(t, resp, 60*time.Second)
	repo := reposWS.Next(t, 10*time.Second, stream.KindRepo, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID, "ImportRepo: import must broadcast a RepoDTO with an id")

	// The adopted main worktree is read back once the import settled: Quiesce
	// drains the projections, then the chat list is read.
	e.Quiesce()
	chats := e.ListChats(t, projectID, repoID)
	var wsID, chatID string
	for _, c := range chats {
		wt, ok := c["worktree"].(map[string]any)
		if !ok {
			continue
		}
		if isDefault, _ := wt["isDefault"].(bool); !isDefault {
			continue
		}
		wsID, _ = c["workspaceId"].(string)
		chatID, _ = c["id"].(string)
		break
	}
	require.NotEmptyf(
		t,
		wsID,
		"ImportRepo: import must adopt the repo's default worktree; chats: %s",
		MustJSON(chats),
	)
	require.NotEmpty(t, chatID, "ImportRepo: the adopted worktree must be born under a chat")
	return ImportedRepo{
		ProjectID:   projectID,
		RepoID:      repoID,
		WorkspaceID: wsID,
		ChatID:      chatID,
		RepoPath:    path,
	}
}

// RecordRepoDeleteIntent writes the durable intent a repo delete records
// before any teardown — the state a crash right after that write leaves.
func (e *Env) RecordRepoDeleteIntent(
	t *testing.T,
	repoID string,
) {
	t.Helper()
	ctx := context.Background()
	repo, err := e.app.GORM.Repositories.FindByKey(ctx, repoID)
	require.NoError(t, err)
	require.NotNil(t, repo, "RecordRepoDeleteIntent: repo %s", repoID)
	repo.Deleting = true
	require.NoError(t, e.app.GORM.Repositories.Save(ctx, *repo))
}

// RepoRow reports whether repoID's row is still in the repository store.
func (e *Env) RepoRow(
	t *testing.T,
	repoID string,
) bool {
	t.Helper()
	repo, err := e.app.GORM.Repositories.FindByKey(context.Background(), repoID)
	require.NoError(t, err)
	return repo != nil
}
