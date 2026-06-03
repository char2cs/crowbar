//go:build integration

package kanban

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestKanban(t *testing.T) { suite.Run(t, new(KanbanSuite)) }

type KanbanSuite struct{ kit.IntegrationSuite }

// TestCreateItemInImplementation verifies that an agent can create a kanban
// item via MCP tools in a state where items tracking is enabled, that the item
// appears in ListKanbanItems, and that a WebSocket event is delivered.
func (s *KanbanSuite) TestCreateItemInImplementation() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "kanban-impl-branch")

	// Advance to implementation state via stub signals.
	// brainstorming → spec
	token := env.Stub.WaitForRunT(s.T(), ctx)
	require.NoError(s.T(), env.MCPClient.WithTest(s.T()).WithToken(token).Signal("user_approved", ""))
	// spec → implementation
	token = env.Stub.WaitForRunT(s.T(), ctx)
	require.NoError(s.T(), env.MCPClient.WithTest(s.T()).WithToken(token).Signal("user_approved", ""))

	// Now in implementation; get the active token.
	token = env.Stub.WaitForRunT(s.T(), ctx)
	mcp := env.MCPClient.WithTest(s.T()).WithToken(token)

	// Subscribe to kanban item events.
	itemCh, closeItems := env.WSClient.SubscribeKanbanItems(task.ID)
	defer closeItems()

	itemID, err := mcp.CreateItem("add login button")
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), itemID)

	// Assert item appears in REST list.
	items := env.Client.ListKanbanItems(task.ID)
	require.Len(s.T(), items, 1)
	assert.Equal(s.T(), "add login button", items[0].Title)

	// Assert WebSocket event delivered.
	waitKanbanItem(s.T(), itemCh, itemID, 3*time.Second)

	// Update status.
	require.NoError(s.T(), mcp.UpdateItemStatus(itemID, "implementing"))

	updatedItems := env.Client.ListKanbanItems(task.ID)
	require.Len(s.T(), updatedItems, 1)
	assert.Equal(s.T(), "implementing", updatedItems[0].Status)

	// Assert status update WebSocket event.
	waitKanbanItemStatus(s.T(), itemCh, itemID, "implementing", 3*time.Second)
}

// TestCreateItemForbiddenInBrainstorming verifies that crowbar_create_item
// returns a forbidden error when the task is in a state with items=false.
func (s *KanbanSuite) TestCreateItemForbiddenInBrainstorming() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	kit.DefaultTask(env.Client, repo.ID, "kanban-brainstorm-branch")

	// Task is in brainstorming; items=false.
	token := env.Stub.WaitForRunT(s.T(), ctx)
	mcp := env.MCPClient.WithTest(s.T()).WithToken(token)

	_, err := mcp.CreateItem("should fail")
	assert.Error(s.T(), err, "expected error creating item in brainstorming state")
}

// waitKanbanItem blocks until an item with the given ID is delivered on ch.
func waitKanbanItem(
	t *testing.T,
	ch <-chan domain.KanbanItem,
	itemID string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("waitKanbanItem: timeout waiting for item %s", itemID)
			return
		}
		select {
		case <-time.After(remaining):
			t.Fatalf("waitKanbanItem: timeout waiting for item %s", itemID)
			return
		case item, ok := <-ch:
			if !ok {
				t.Fatalf("waitKanbanItem: channel closed")
				return
			}
			if item.ID == itemID {
				return
			}
		}
	}
}

// waitKanbanItemStatus blocks until an item with the given ID and status is
// delivered on ch.
func waitKanbanItemStatus(
	t *testing.T,
	ch <-chan domain.KanbanItem,
	itemID string,
	status string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("waitKanbanItemStatus: timeout waiting for item %s status %s", itemID, status)
			return
		}
		select {
		case <-time.After(remaining):
			t.Fatalf("waitKanbanItemStatus: timeout")
			return
		case item, ok := <-ch:
			if !ok {
				return
			}
			if item.ID == itemID && item.Status == status {
				return
			}
		}
	}
}
