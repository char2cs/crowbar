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

// TestRegression_RepoChatsListCarriesNonChatRowType pins the two facts Task 1
// of the 2026-09-01 owning-chat-backfill plan exists to close: ListChatsInRepo
// does not exclude a row by its Type (its only type-based exclusion is
// ChatTypeFolder), and AgentChatDTO carries the row's own Type onto the wire —
// so a client can tell a non-default-typed row apart from an ordinary chat
// within the same list response.
//
// ChatTypeWorkflow stands in for the non-default type under test — a
// forward-compat marker with no v0 writer, but still one of the two types
// validChatType accepts (2026-09-08 sidebar-placement-unification Task 9
// narrowed that set to exclude ChatTypeBranch, this test's original stand-in:
// a workspace's own position is a Node{Kind:workspace} row now, never a
// retyped Chat, so nothing can mint one to seed here any more).
func TestRegression_RepoChatsListCarriesNonChatRowType(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)

	workflowID := "workflow-" + imported.workspaceID
	_, err := h.app.Repositories.AgentChat.Create(context.Background(), agentchat.CreateInput{
		ID:          workflowID,
		WorkspaceID: imported.workspaceID,
		Type:        domain.ChatTypeWorkflow,
		Now:         time.Now(),
	})
	require.NoError(t, err)

	var rows []map[string]any
	h.get(repoBase(imported)+"/chats", &rows)

	var found map[string]any
	for _, row := range rows {
		if row["id"] == workflowID {
			found = row
			break
		}
	}
	require.NotNil(t, found, "the workflow row must be listed under its repo, not dropped like a folder")
	assert.Equal(t, "workflow", found["type"], "the wire row must carry the row's own Type")
}
