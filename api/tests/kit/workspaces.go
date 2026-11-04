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

// WorktreePath returns the server's own persisted worktree path from the read model rather than
// re-deriving the slug (worktreepath is unreachable from the kit). projectID/repoID are unused;
// the path is keyed by wsID.
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

// ProviderState is a scripted provider poll result for PushProviderState, mirroring the
// aggregate's inputs without depending on the repository's ProviderInput.
type ProviderState struct {
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
}

// WorktreeFrame flattens a REST chat row's `worktree` object, restoring `id`, `chatId` and
// `repoId` from the row; a row with no worktree is returned unchanged.
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

// WaitForWorkspace reads the projects scope until the workspace entity wsID
// satisfies pred, returning its row.
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

// WaitForWorkspaceState reads the projects scope until workspace wsID reaches
// status. A blank status matches the omitempty "" state; "deleted" matches the
// gone frame a tombstoned workspace is sent as.
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

// CreateWorkspace creates a worktree on a caller-named branch through the USECASE: naming the
// branch is a fixture need no HTTP route offers. It makes the live import path's CreateChild
// call, minting and attaching the owning chat itself (see createWorkspace).
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

// CreateChildWorkspace creates a child workspace under parentID (a WORKSPACE
// id) and returns the UUID. See CreateWorkspace for why this is a usecase call.
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

// createWorkspace mints and attaches the owning chat via the import paths' three-verb
// primitive: nothing would give an orphaned CreateChild call one afterwards.
func (e *Env) createWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) (wsID, chatID string) {
	t.Helper()
	wsID, chatID = e.createWorkspaceWrites(t, projectID, repoID, branch, parentID)
	// The create's own writes are asynx commands; drain them so a caller that
	// reads the row (or dials its chat) next sees it rather than racing it.
	e.Quiesce()
	return wsID, chatID
}

// createWorkspaceWrites issues a create's commands inside the kit's write
// window: a Quiesce from a sibling goroutine waits for them instead of making
// the dispatcher refuse them (see Env.quiesceMu).
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
	// ParentBranch must be supplied: resolveInherited only defaults it when neither RepoID nor
	// RepoPath is named, and a blank one makes `git worktree add` die.
	parentBranch := repo.DefaultBranch
	if parentID != "" {
		parent, perr := e.app.Repositories.Workspace.Get(ctx, parentID)
		require.NoError(t, perr, "createWorkspace: read parent workspace %s", parentID)
		parentBranch = parent.Branch
	}

	mintedChatID, err := e.app.Usecases.AgentChatFolder.MintOwningChat(ctx, "", parentID)
	require.NoError(t, err, "createWorkspace: mint the owning chat")

	// Forced true for the same reason the live import path forces it: the
	// taxonomy default is "inherit whether the parent owns a worktree", and a
	// fixture asking for a branch is asking for a real worktree on it.
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

// CreateWorkspaceWithChat is CreateWorkspace plus the id of the chat the new
// worktree is born under — the pair almost every chat-scoped test needs, in one
// call instead of a create followed by a resolve.
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

// DeleteWorkspaceCascade tombstones a workspace and starts its purge cascade, for a test that
// needs a tombstone as a PRECONDITION: DELETE .../chats/:id also purges the owning chat.
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

// WorkspaceRow returns a workspace's status in the DURABLE read model and whether its row exists.
// In-process because no wire read reaches a row whose owning chat was purged; it reads the same
// store the boot orphan-sweep lists from.
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

// PushProviderState injects a scripted provider poll result into the workspace aggregate,
// bypassing the provider engine; the transition is persisted (SendWait) and broadcast, so tests
// drive PR-status deterministically.
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
