//go:build integration

package v0_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	chatrepo "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// readUntil loop-reads from conn until match returns true for a decoded message,
// then returns that message. The broadcast may be preceded by other projected
// rows on the same topic, so the loop skips non-matching frames.
//
// The reads block: each frame's arrival IS the signal, so the loop advances
// exactly as fast as the daemon delivers, with no deadline to outrun under load.
// A matching frame that never arrives hangs until `go test -timeout` fires and
// dumps the goroutines, naming this test.
func readUntil(
	t *testing.T,
	conn *websocket.Conn,
	match func(map[string]any) bool,
) map[string]any {
	t.Helper()
	for {
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(msg, &got))
		if match(got) {
			return got
		}
	}
}

// TestWave3_WorkspaceCommand_ReachesChatWSClient proves the FULL Wave 3 chain:
// a real Asynx command on the Workspace aggregate (repo Create /
// SyncWorkingTreeState, both SendWait) -> the read-model projection registered in
// app.New fires -> hub.BroadcastWorkspace -> PushWorkspace -> pushChatWorktree ->
// the agent-chat broadcaster -> a connected WS client.
//
// The chat row is created BEFORE the v0 container exists, for two reasons: the
// projection resolves the owning chat off the read model (enrichFrame), and its
// own creation frame would otherwise land on the very stream under test.
func TestWave3_WorkspaceCommand_ReachesChatWSClient(t *testing.T) {
	tc := newApp(t)
	seedRepo(t, tc, "r1")
	ctx := context.Background()
	now := time.Unix(1, 0).UTC()

	_, err := tc.app.Repositories.AgentChat.Create(ctx, chatrepo.CreateInput{
		ID: "chat-1", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: now,
	})
	require.NoError(t, err)

	c, srv := serveAgentChats(t, tc)
	conn := dialV0(t, srv, "/v0/chats/chat-1/ws")
	c.WaitAgentChatsRegistered()

	// Command 1: Create. SendWait blocks until the command + read-model projection
	// cycle completes, so the broadcast has already fired when this returns.
	_, err = tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "feat/x"},
		now,
	)
	require.NoError(t, err)

	created := readUntil(t, conn, func(m map[string]any) bool {
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

	updated := readUntil(t, conn, func(m map[string]any) bool {
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
