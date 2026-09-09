//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestRegression_WorkspaceDTOResolvesItsOwningChatWithoutAnyBackfill pins
// what replaces the boot backfill this file used to pin (2026-09-08
// sidebar-placement-unification Task 9 deleted BackfillOwningChats,
// EnsureOwningChat and the machinery behind them): a workspace's owning chat
// is now guaranteed chat-first, at creation
// (MintOwningChat/AttachOwningWorkspace, owning_chat.go), and nothing ever
// reconciles a workspace afterward — this daemon boots exactly once here,
// and the workspace under test is never touched by anything but its own
// creation.
//
// The ambiguity domain.ResolveOwningChat exists to resolve still arises in
// the fresh system, just from a different source than a boot backfill's
// adopt-or-mint: a workspace's worktree is many-chats-to-one, so a SECOND,
// ordinary conversation started inside a workspace shares its WorkspaceID
// with the row that already owns it. The wire owningChatId must still
// resolve to the ORIGINAL chat-first row — the id every chat-scoped route
// addresses that worktree through — never to the later conversation, which
// this test plants directly against the repository (a real conversation
// reaches the identical row shape through CreateChat's plain-thread path;
// this fixture only needs the ROW, never the CLI a live spawn would start).
func TestRegression_WorkspaceDTOResolvesItsOwningChatWithoutAnyBackfill(t *testing.T) {
	h := newHarnessAt(t, t.TempDir())
	imported := importWritableWorkspace(t, h)

	_, err := h.app.Repositories.AgentChat.Create(context.Background(), agentchat.CreateInput{
		ID:          "second-conversation",
		WorkspaceID: imported.workspaceID,
		Type:        domain.ChatTypeChat,
		Now:         time.Now().UTC(),
	})
	require.NoError(t, err)
	h.Quiesce()

	rows, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(context.Background(), imported.workspaceID)
	require.NoError(t, err)
	require.Len(t, rows, 2, "the workspace's chat-first row and the later conversation both name it")

	var owningChatID string
	for _, c := range listChats(t, h, imported.projectID, imported.repoID) {
		if c.WorkspaceID == imported.workspaceID {
			require.NotNil(t, c.Worktree, "every row naming this workspace must carry its worktree fields")
			owningChatID = c.Worktree.OwningChatID
		}
	}
	assert.Equal(t, imported.chatID, owningChatID,
		"the wire owningChatId must resolve to the chat-first minted row, never the later conversation, "+
			"and never through a boot backfill -- this task deleted the last one")
}
