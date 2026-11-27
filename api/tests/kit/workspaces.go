//go:build integration

package kit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	wsrepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func (e *Env) WorktreePath(
	_ string,
	_ string,
	wsID string,
) string {
	ws, err := e.app.Repositories.Workspace.Get(context.Background(), wsID)
	if err != nil {
		panic(fmt.Sprintf("kit: WorktreePath: get workspace %q: %v", wsID, err))
	}
	return ws.WorktreePath
}

type ProviderState struct {
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
}

func WorktreeFrame(
	m map[string]any,
) map[string]any {
	wt, ok := m["worktree"].(map[string]any)
	if !ok {
		return m
	}
	wsID, _ := m["workspaceId"].(string)
	if wsID == "" {
		return m
	}
	out := make(map[string]any, len(wt)+3)
	for k, v := range wt {
		out[k] = v
	}
	out["id"] = wsID
	out["chatId"] = m["chatId"]
	if repoID, ok := m["repoId"]; ok {
		out["repoId"] = repoID
	}
	return out
}

func WaitForWorkspace(
	t *testing.T,
	w *EntityWatcher,
	wsID string,
	timeout time.Duration,
	pred func(map[string]any) bool,
) map[string]any {
	t.Helper()
	return w.Next(t, timeout, stream.KindWorkspace, func(row map[string]any) bool {
		return row["id"] == wsID && pred(row)
	})
}

func WaitForWorkspaceState(
	t *testing.T,
	w *EntityWatcher,
	wsID string,
	status string,
	timeout time.Duration,
) map[string]any {
	t.Helper()
	return WaitForWorkspace(t, w, wsID, timeout, func(row map[string]any) bool {
		if gone, _ := row["gone"].(bool); gone {
			return status == string(domain.WorkspaceStatusDeleted)
		}
		got, _ := row["status"].(string)
		return got == status
	})
}

func (e *Env) CreateWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
) string {
	t.Helper()
	wsID, _ := e.createWorkspace(t, projectID, repoID, branch, "")
	return wsID
}

func (e *Env) CreateChildWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) string {
	t.Helper()
	wsID, _ := e.createWorkspace(t, projectID, repoID, branch, parentID)
	return wsID
}

func (e *Env) createWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) (wsID, chatID string) {
	t.Helper()
	wsID, chatID = e.createWorkspaceWrites(t, projectID, repoID, branch, parentID)
	e.Quiesce()
	return wsID, chatID
}

func (e *Env) createWorkspaceWrites(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) (wsID, chatID string) {
	t.Helper()
	e.quiesceMu.RLock()
	defer e.quiesceMu.RUnlock()
	ctx := context.Background()
	repo, err := e.app.GORM.Repositories.FindByKey(ctx, repoID)
	require.NoError(t, err, "createWorkspace: read repo %s", repoID)
	require.NotNil(t, repo, "createWorkspace: repo %s must exist", repoID)
	parentBranch := repo.DefaultBranch
	if parentID != "" {
		parent, perr := e.app.Repositories.Workspace.Get(ctx, parentID)
		require.NoError(t, perr, "createWorkspace: read parent workspace %s", parentID)
		parentBranch = parent.Branch
	}

	mintedChatID, err := e.app.Usecases.AgentChatFolder.MintOwningChat(ctx, "", parentID)
	require.NoError(t, err, "createWorkspace: mint the owning chat")

	ownWorktree := true
	ws, err := e.app.Usecases.Workspace.CreateChild(ctx, wsusecase.CreateChildInput{
		RepoID:       repoID,
		ProjectID:    projectID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       branch,
		ParentID:     parentID,
		ParentBranch: parentBranch,
		OwnWorktree:  &ownWorktree,
	})
	if err != nil {
		require.NoError(t, e.app.Usecases.AgentChatFolder.DiscardOwningChat(ctx, mintedChatID),
			"createWorkspace: discard the owning chat of a workspace that was never created")
	}
	require.NoErrorf(t, err, "createWorkspace: create %q under %q", branch, parentID)
	require.NotEmpty(t, ws.ID, "createWorkspace: the created workspace must carry an id")

	require.NoError(t, e.app.Usecases.AgentChatFolder.AttachOwningWorkspace(ctx, mintedChatID, ws),
		"createWorkspace: attach the minted chat to its workspace")
	return ws.ID, mintedChatID
}

func (e *Env) CreateWorkspaceWithChat(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) (string, string) {
	t.Helper()
	return e.createWorkspace(t, projectID, repoID, branch, parentID)
}

func (e *Env) DeleteWorkspaceCascade(
	t *testing.T,
	wsID string,
) {
	t.Helper()
	require.NoError(
		t,
		e.app.Usecases.Workspace.DeleteCascade(context.Background(), wsID, domain.KeepWorkAtRisk),
		"DeleteWorkspaceCascade: %s",
		wsID,
	)
}

func (e *Env) WorkspaceRow(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) (status string, present bool) {
	t.Helper()
	rows, err := e.app.Repositories.Workspace.ListInRepo(context.Background(), projectID, repoID)
	require.NoError(t, err, "WorkspaceRow: list %s/%s", projectID, repoID)
	for _, w := range rows {
		if w.ID == wsID {
			return string(w.Status), true
		}
	}
	return "", false
}

func (e *Env) PushProviderState(
	t *testing.T,
	wsID string,
	state ProviderState,
) {
	t.Helper()
	_, err := e.app.Repositories.Workspace.SyncProviderState(
		context.Background(),
		wsrepo.ProviderInput{
			ID:             wsID,
			Protected:      state.Protected,
			HasPR:          state.HasPR,
			PRStatus:       state.PRStatus,
			PRUrl:          state.PRUrl,
			PRTitle:        state.PRTitle,
			PRTargetBranch: state.PRTargetBranch,
		},
		Now(),
	)
	require.NoError(t, err, "PushProviderState: SyncProviderState")
}
