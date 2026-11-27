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
