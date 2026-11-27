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
