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

// TestRegression_RepoChatsListCarriesBranchRowType pins the two facts Task 1
// of the 2026-09-01 owning-chat-backfill plan exists to close: ListChatsInRepo
// already includes ChatTypeBranch rows in what it returns to
// GET /repos/:rid/chats (its only type-based exclusion is ChatTypeFolder), and
// AgentChatDTO now actually carries the row's own Type onto the wire — so a
// client can finally tell a locked-branch/repo-home row apart from an ordinary
// chat within the same list response.
//
// A ChatTypeBranch row cannot yet be minted over the HTTP surface (that is
// Task 3's backfill), so this seeds one directly against the AgentChat
// aggregate, owning the workspace the import just created — exactly the shape
// a backfilled row will have.
func TestRegression_RepoChatsListCarriesBranchRowType(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)

	branchID := "branch-" + imported.workspaceID
	_, err := h.app.Repositories.AgentChat.Create(context.Background(), agentchat.CreateInput{
		ID:          branchID,
		WorkspaceID: imported.workspaceID,
		Type:        domain.ChatTypeBranch,
		Now:         time.Now(),
	})
	require.NoError(t, err)

	var rows []map[string]any
	h.get(repoBase(imported)+"/chats", &rows)

	var found map[string]any
	for _, row := range rows {
		if row["id"] == branchID {
			found = row
			break
		}
	}
	require.NotNil(t, found, "the branch row must be listed under its repo, not dropped like a folder")
	assert.Equal(t, "branch", found["type"], "the wire row must carry the row's own Type")
}
