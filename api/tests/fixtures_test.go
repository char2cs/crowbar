//go:build integration

package tests

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type importedRepo struct {
	projectID   string
	repoID      string
	workspaceID string
	chatID      string
	repoPath    string
}

type chatRow struct {
	ID          string        `json:"id"`
	WorkspaceID string        `json:"workspaceId"`
	Type        string        `json:"type"`
	Worktree    *worktreeWire `json:"worktree"`
}

type worktreeWire struct {
	Branch          string `json:"branch"`
	Status          string `json:"status"`
	Working         bool   `json:"working"`
	IsDefault       bool   `json:"isDefault"`
	Added           int    `json:"added"`
	Deleted         int    `json:"deleted"`
	MergeStrategy   string `json:"mergeStrategy"`
	CanMergeLocally bool   `json:"canMergeLocally"`
	MergeConflicts  bool   `json:"mergeConflicts"`
	ParentBranch    string `json:"parentBranch"`
	PRUrl           string `json:"prUrl"`
	PRTitle         string `json:"prTitle"`
	PRTargetBranch  string `json:"prTargetBranch"`
	LocalPath       string `json:"localPath"`
	HeldByPath      string `json:"heldByPath"`
	ForkPointSha    string `json:"forkPointSha"`
	ParentID        string `json:"parentId"`
	OwningChatID    string `json:"owningChatId"`
}

func listChats(
	t *testing.T,
	h *harness,
	projectID string,
	repoID string,
) []chatRow {
	t.Helper()
	var chats []chatRow
	h.get("/v0/projects/"+projectID+"/repos/"+repoID+"/chats", &chats)
	return chats
}

func importProject(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	repoPath := gitRepoWithCommit(t)
	runGit(t, repoPath, "checkout", "--detach")

	projectID, repoID := createProjectAndRepo(t, h, repoPath)

	h.Quiesce()
	chats := listChats(t, h, projectID, repoID)
	var wsID, chatID string
	for _, c := range chats {
		if c.Worktree == nil || c.Worktree.Branch != "main" || c.Worktree.HeldByPath != "" {
			continue
		}
		wsID, chatID = c.WorkspaceID, c.ID
		break
	}
	require.NotEmpty(t, wsID, "import must provision the main managed worktree")
	require.NotEmpty(t, chatID, "the main managed worktree must be born under a chat")

	return importedRepo{
		projectID:   projectID,
		repoID:      repoID,
		workspaceID: wsID,
		chatID:      chatID,
		repoPath:    repoPath,
	}
}

func importProjectHomeHoldsDefault(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	repoPath := gitRepoWithCommit(t)

	projectID, repoID := createProjectAndRepo(t, h, repoPath)

	h.Quiesce()
	var homeID, homeChatID string
	sawPlaceholder := false
	for _, c := range listChats(t, h, projectID, repoID) {
		if c.Worktree == nil {
			continue
		}
		if c.Worktree.IsDefault {
			homeID, homeChatID = c.WorkspaceID, c.ID
		}
		if c.Worktree.Branch == "main" && c.Worktree.HeldByPath != "" {
			sawPlaceholder = true
		}
	}
	require.NotEmpty(t, homeID, "import must adopt the home as the isDefault workspace")
	require.True(t, sawPlaceholder, "the held default branch must surface as a placeholder row")

	return importedRepo{
		projectID:   projectID,
		repoID:      repoID,
		workspaceID: homeID,
		chatID:      homeChatID,
		repoPath:    repoPath,
	}
}

func createProjectAndRepo(
	t *testing.T,
	h *harness,
	repoPath string,
) (projectID, repoID string) {
	t.Helper()
	projectsWS := watchEntities(t, h, stream.KindProject)
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "demo", "path": repoPath}, http.StatusAccepted)
	_ = resp.Body.Close()

	project := projectsWS.read(t, func(m map[string]any) bool {
		return m["path"] == repoPath
	})
	projectID, _ = project["id"].(string)
	require.NotEmpty(t, projectID, "create must broadcast a ProjectDTO with an id")

	reposWS := watchEntities(t, h, stream.KindRepo)
	importRepo(t, h, "/v0/projects/"+projectID+"/repos", map[string]string{"name": "demo", "path": repoPath})
	repo := reposWS.read(t, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ = repo["id"].(string)
	require.NotEmpty(t, repoID, "add-repo must broadcast a RepoDTO with an id")
	return projectID, repoID
}

func importRepo(t *testing.T, h *harness, path string, body map[string]string) {
	t.Helper()
	awaitOperation(t, h, startOperation(t, h, path, body).ID, operationSettled)
}

func importWritableWorkspace(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	imported := importProject(t, h)
	imported.workspaceID, imported.chatID = createWorktree(t, h, imported, "feature/write", imported.workspaceID)
	return imported
}

func createWorktree(
	t *testing.T,
	h *harness,
	imported importedRepo,
	branch string,
	parentID string,
) (wsID, chatID string) {
	t.Helper()
	ctx := context.Background()

	repo, err := h.app.GORM.Repositories.FindByKey(ctx, imported.repoID)
	require.NoError(t, err, "createWorktree: read repo")
	require.NotNil(t, repo, "createWorktree: repo must exist")

	parentBranch := repo.DefaultBranch
	if parentID != "" {
		parent, perr := h.app.Repositories.Workspace.Get(ctx, parentID)
		require.NoError(t, perr, "createWorktree: read parent workspace")
		parentBranch = parent.Branch
	}

	mintedChatID, err := h.app.Usecases.AgentChatFolder.MintOwningChat(ctx, "", parentID)
	require.NoError(t, err, "createWorktree: mint the owning chat")

	ownWorktree := true
	ws, err := h.app.Usecases.Workspace.CreateChild(ctx, wsusecase.CreateChildInput{
		RepoID:       imported.repoID,
		ProjectID:    imported.projectID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       branch,
		ParentID:     parentID,
		ParentBranch: parentBranch,
		OwnWorktree:  &ownWorktree,
	})
	if err != nil {
		require.NoError(t, h.app.Usecases.AgentChatFolder.DiscardOwningChat(ctx, mintedChatID),
			"createWorktree: discard the owning chat of a workspace that was never created")
	}
	require.NoErrorf(t, err, "createWorktree: create %q under %q", branch, parentID)

	require.NoError(t, h.app.Usecases.AgentChatFolder.AttachOwningWorkspace(ctx, mintedChatID, ws),
		"createWorktree: attach the minted chat to its workspace")

	h.Quiesce()

	return ws.ID, mintedChatID
}

func createChildWorkspace(
	t *testing.T,
	h *harness,
	imported importedRepo,
	branch string,
	parentID string,
) string {
	t.Helper()
	wsID, _ := createWorktree(t, h, imported, branch, parentID)
	return wsID
}

func worktreeOf(
	t *testing.T,
	h *harness,
	imported importedRepo,
	wsID string,
) workspaceDTO {
	t.Helper()
	for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
		if w.ID == wsID {
			return w
		}
	}
	require.Failf(t, "workspace not found", "no chat row holds workspace %s", wsID)
	return workspaceDTO{}
}

func owningChatID(
	t *testing.T,
	h *harness,
	wsID string,
) string {
	t.Helper()
	ctx := context.Background()
	h.Quiesce()
	rows, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, wsID)
	require.NoError(t, err, "owningChatID: list the chats holding %s", wsID)
	owner, ok := domain.ResolveOwningChat(rows)
	require.Truef(t, ok, "workspace %s must be held by an owning chat", wsID)
	return owner.ID
}

type repoDTO struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Path      string `json:"path"`
}

func listRepos(
	t *testing.T,
	h *harness,
	projectID string,
) []repoDTO {
	t.Helper()
	var repos []repoDTO
	h.get("/v0/projects/"+projectID+"/repos", &repos)
	return repos
}

type workspaceDTO struct {
	ID              string `json:"id"`
	RepoID          string `json:"repoId"`
	ProjectID       string `json:"projectId"`
	Branch          string `json:"branch"`
	ParentID        string `json:"parentId"`
	ForkPointSha    string `json:"forkPointSha"`
	IsDefault       bool   `json:"isDefault"`
	Status          string `json:"status"`
	Working         bool   `json:"working"`
	Added           int    `json:"added"`
	Deleted         int    `json:"deleted"`
	MergeStrategy   string `json:"mergeStrategy"`
	CanMergeLocally bool   `json:"canMergeLocally"`
	ParentBranch    string `json:"parentBranch"`
	PRUrl           string `json:"prUrl"`
	PRTitle         string `json:"prTitle"`
	PRTargetBranch  string `json:"prTargetBranch"`
	LocalPath       string `json:"localPath"`
	HeldByPath      string `json:"heldByPath"`
	OwningChatID    string `json:"owningChatId"`
}

func listWorkspaces(
	t *testing.T,
	h *harness,
	projectID string,
	repoID string,
) []workspaceDTO {
	t.Helper()
	var workspaces []workspaceDTO
	for _, c := range listChats(t, h, projectID, repoID) {
		if c.Worktree == nil || c.WorkspaceID == "" {
			continue
		}
		w := c.Worktree
		workspaces = append(workspaces, workspaceDTO{
			ID:              c.WorkspaceID,
			RepoID:          repoID,
			ProjectID:       projectID,
			Branch:          w.Branch,
			ParentID:        w.ParentID,
			ForkPointSha:    w.ForkPointSha,
			IsDefault:       w.IsDefault,
			Status:          w.Status,
			Working:         w.Working,
			Added:           w.Added,
			Deleted:         w.Deleted,
			MergeStrategy:   w.MergeStrategy,
			CanMergeLocally: w.CanMergeLocally,
			ParentBranch:    w.ParentBranch,
			PRUrl:           w.PRUrl,
			PRTitle:         w.PRTitle,
			PRTargetBranch:  w.PRTargetBranch,
			LocalPath:       w.LocalPath,
			HeldByPath:      w.HeldByPath,
			OwningChatID:    c.ID,
		})
	}
	return workspaces
}

func wsBase(
	imported importedRepo,
) string {
	return "/v0/chats/" + imported.chatID
}

func threadsBase(
	imported importedRepo,
) string {
	return "/v0/projects/" + imported.projectID +
		"/repos/" + imported.repoID +
		"/workspaces/" + imported.workspaceID
}

const documentsContent = "/v0/documents/content"

func docURL(
	wsID string,
	path string,
) string {
	return documentsContent + "?" + url.Values{"workspace": {wsID}, "path": {path}}.Encode()
}

func writeFile(
	dir string,
	name string,
	content string,
) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
}
