//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The pre-audit daemon recorded a workspace's owner lazily: the first read
// that listed the workspace picked a row by heuristic and recorded it. A
// workspace it never listed reaches the upgrade with its owner unrecorded.
// Boot must record that same row — the conversation the user forked the branch
// from, or the legacy branch row — not mint an empty chat that hijacks the
// workspace's sidebar level.
func TestRegression_Upgrade_BootRecordsTheLegacyOwnerInsteadOfMintingOne(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importProject(t, h)
	h.Quiesce()
	h.crash()

	now := time.Now().UTC()
	fork := domain.Workspace{
		ID: uuid.NewString(), RepoID: imported.repoID, ProjectID: imported.projectID,
		Branch: "feature/login", ParentID: imported.workspaceID, MergeStrategy: "merge",
		CreatedAt: now, LastActivity: now,
	}
	locked := domain.Workspace{
		ID: uuid.NewString(), RepoID: imported.repoID, ProjectID: imported.projectID,
		Branch: "release", Status: domain.WorkspaceStatusLocked, MergeStrategy: "merge",
		CreatedAt: now, LastActivity: now,
	}
	forker := domain.Chat{ID: uuid.NewString(), Type: domain.ChatTypeChat, WorkspaceID: fork.ID,
		Title: "Fix login", CreatedAt: now, LastActivityAt: now}
	thread := domain.Chat{ID: uuid.NewString(), Type: domain.ChatTypeChat, WorkspaceID: fork.ID,
		ParentID: forker.ID, CreatedAt: now.Add(-time.Minute), LastActivityAt: now}
	conversation := domain.Chat{ID: uuid.NewString(), Type: domain.ChatTypeChat, WorkspaceID: locked.ID,
		CreatedAt: now.Add(-time.Hour), LastActivityAt: now}
	branchRow := domain.Chat{ID: uuid.NewString(), Type: domain.ChatTypeBranch, WorkspaceID: locked.ID,
		CreatedAt: now, LastActivityAt: now}
	seed := openBaseEra(t, home)
	seed.workspace(fork)
	seed.workspace(locked)
	for _, c := range []domain.Chat{forker, thread, conversation, branchRow} {
		seed.chat(c)
	}
	seed.close()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	for ws, want := range map[string]domain.Chat{fork.ID: forker, locked.ID: branchRow} {
		chats, err := h2.app.Usecases.AgentChat.ListChatsByWorkspace(context.Background(), ws)
		require.NoError(t, err)
		assert.Len(t, chats, 2, "no chat is minted for a workspace that already has its owner")
		for _, c := range chats {
			assert.Equal(t, c.ID == want.ID, c.OwnsWorkspace, "chat %s of %s", c.ID, ws)
		}
	}
}
