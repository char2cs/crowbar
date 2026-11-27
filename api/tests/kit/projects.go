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

type ImportedRepo struct {
	ProjectID   string
	RepoID      string
	WorkspaceID string
	ChatID      string
	RepoPath    string
}

func (e *Env) ImportRepo(
	t *testing.T,
	name string,
	path string,
) ImportedRepo {
	t.Helper()
	if path == "" {
		path = InitRepo(t)
	}
	t.Cleanup(func() { e.Close(t) })
	projectID := e.RegisterProject(t, name, path)
	reposWS := e.WatchProjects(t)
	resp := e.POST(t, "/v0/projects/"+projectID+"/repos", map[string]any{ //nolint:bodyclose // AwaitOperation closes the body
		"name": name,
		"path": path,
	})
	e.AwaitOperation(t, resp, 60*time.Second)
	repo := reposWS.Next(t, 10*time.Second, stream.KindRepo, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID, "ImportRepo: import must broadcast a RepoDTO with an id")

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

func (e *Env) RepoRow(
	t *testing.T,
	repoID string,
) bool {
	t.Helper()
	repo, err := e.app.GORM.Repositories.FindByKey(context.Background(), repoID)
	require.NoError(t, err)
	return repo != nil
}
