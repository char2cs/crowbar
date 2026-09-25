//go:build integration

package v0_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	chatrepo "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestWave3_WorkspaceCommand_ReachesChatWSClient proves the FULL Wave 3 chain:
// a real Asynx command on the Workspace aggregate (repo Create /
// SyncWorkingTreeState, both SendWait) -> the read-model projection registered in
// app.New fires -> hub.BroadcastWorkspace -> PushWorkspace -> pushChatWorktree ->
// the agent-chat broadcaster -> a connected WS client.
//
// The owning chat is minted and attached BEFORE the v0 container exists, the
// way chat-first creation does it: the projection resolves the owner off the
// recorded Chat.OwnsWorkspace (enrichFrame), and the chat's own frames would
// otherwise land on the very stream under test.
func TestWave3_WorkspaceCommand_ReachesChatWSClient(t *testing.T) {
	tc := newApp(t)
	seedRepo(t, tc, "r1")
	ctx := context.Background()
	now := time.Unix(1, 0).UTC()

	_, err := tc.app.Repositories.AgentChat.Create(ctx, chatrepo.CreateInput{
		ID: "chat-1", Type: domain.ChatTypeChat, Now: now,
	})
	require.NoError(t, err)
	_, err = tc.app.Repositories.AgentChat.SetWorkspace(ctx, "chat-1", "w1")
	require.NoError(t, err)

	c, srv := serveAgentChats(t, tc)
	conn := dialV0(t, srv, "/v0/chats/chat-1/ws")
	c.WaitAgentChatsRegistered()

	// Command 1: Create. SendWait blocks until the command + read-model projection
	// cycle completes, so the broadcast has already fired when this returns.
	_, err = tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "feat/x", Provisioning: domain.WorkspacePlaceholder},
		now,
	)
	require.NoError(t, err)

	created := conn.ReadUntil(t, wsReadBound, func(m map[string]any) bool {
		return m["kind"] == dto.AgentChatKindWorktreeState && m["workspaceId"] == "w1"
	})
	assert.Equal(t, "chat-1", created["chatId"])
	assert.Equal(t, "r1", created["repoId"])
	createdWorktree := chatWorktreeOf(t, created)
	assert.Equal(t, "feat/x", createdWorktree["branch"])
	// A freshly created workspace carries the "new" status badge.
	assert.Equal(t, "new", createdWorktree["status"])

	// Command 2: SyncWorkingTreeState — a git working-tree summary mutation. The
	// added/deleted counts are recorded; this proves a git-summary mutation
	// propagates the UPDATED row over WS. Per D4 the status STAYS "new" after
	// HasCommits (commits no longer clear the badge — "new" is a first-class
	// lifecycle status, not a transient "no commits yet" hint).
	_, err = tc.app.Repositories.Workspace.SyncWorkingTreeState(
		ctx,
		workspace.SyncInput{ID: "w1", Added: 7, Deleted: 2, HasCommits: true},
		now.Add(time.Minute),
	)
	require.NoError(t, err)

	updated := conn.ReadUntil(t, wsReadBound, func(m map[string]any) bool {
		// The updated row reflects the new added count.
		if m["kind"] != dto.AgentChatKindWorktreeState {
			return false
		}
		worktree, ok := m["worktree"].(map[string]any)
		return ok && worktree["added"] == float64(7)
	})
	updatedWorktree := chatWorktreeOf(t, updated)
	assert.Equal(t, float64(7), updatedWorktree["added"])
	assert.Equal(t, float64(2), updatedWorktree["deleted"])
	// Status badge stays "new" after HasCommits (D4): commits do not clear it.
	assert.Equal(t, "new", updatedWorktree["status"], "status must stay new after HasCommits (D4)")
}
