//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Creating a workspace mints its owning chat, writes the row, then attaches
// the two. A daemon that dies between the last two steps leaves a workspace no
// chat owns — and every worktree surface is chat-keyed, so it is unreachable.
// The next boot gives it an owner (invariant D4); reads no longer have to mint
// one on the fly.
func TestRegression_BootOwnsAWorkspaceWhoseCreateCrashedBeforeAttach(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importProject(t, h)

	// The crash window: the row is written, the attach never happens.
	wsID := uuid.NewString()
	_, err := h.app.Repositories.Workspace.Create(context.Background(), workspace.CreateInput{
		ID: wsID, RepoID: imported.repoID, ProjectID: imported.projectID,
		Branch: "feature/orphaned", ParentID: imported.workspaceID,
		Provisioning: domain.WorkspacePlaceholder,
	}, time.Now())
	require.NoError(t, err)
	h.Quiesce()
	h.crash()

	// Read in-process, before any request: no read may be what mints it.
	h2 := newHarnessAt(t, home)
	h2.Quiesce()
	chats, err := h2.app.Usecases.AgentChat.ListChatsByWorkspace(context.Background(), wsID)
	require.NoError(t, err)
	owners := 0
	for _, c := range chats {
		if c.OwnsWorkspace {
			owners++
		}
	}
	require.Equal(t, 1, owners, "the reboot gives the workspace exactly one owning chat")
}
