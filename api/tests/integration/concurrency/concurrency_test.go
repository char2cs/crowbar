//go:build integration

package concurrency

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestConcurrency(t *testing.T) { suite.Run(t, new(ConcurrencySuite)) }

type ConcurrencySuite struct{ kit.IntegrationSuite }

// TestConcurrentTaskCreation creates 10 tasks simultaneously on the same
// repository and asserts all succeed with distinct worktree paths.
func (s *ConcurrencySuite) TestConcurrentTaskCreation() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())

	const n = 10
	type result struct {
		worktreePath string
		err          error
	}
	results := make([]result, n)
	var wg sync.WaitGroup

	for i := range n {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[i].err = fmt.Errorf("panic: %v", r)
				}
			}()
			task := env.Client.CreateTask(repo.ID, kit.CreateTaskRequest{
				Title:      fmt.Sprintf("concurrent-task-%d", i),
				BranchName: fmt.Sprintf("concurrent-branch-%d-%d", i, time.Now().UnixNano()),
				BaseBranch: "main",
				FlowPath:   "",
			})
			results[i].worktreePath = task.WorktreePath
		}()
	}
	wg.Wait()

	worktreePaths := make(map[string]bool, n)
	for i, r := range results {
		require.NoError(s.T(), r.err, "task %d failed", i)
		require.NotEmpty(s.T(), r.worktreePath, "task %d has empty worktree path", i)
		assert.False(s.T(), worktreePaths[r.worktreePath], "duplicate worktree path: %s", r.worktreePath)
		worktreePaths[r.worktreePath] = true
	}
}

// TestConcurrentKanbanItemCreation creates 5 kanban items simultaneously under
// the same AgentRun token and asserts all 5 are persisted without duplicates.
func (s *ConcurrencySuite) TestConcurrentKanbanItemCreation() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "concurrency-kanban-branch")

	// Advance to implementation state.
	mcp := env.MCPClient.WithTest(s.T())
	token := env.Stub.WaitForRunT(s.T(), ctx)
	require.NoError(s.T(), mcp.WithToken(token).Signal("user_approved", ""))
	token = env.Stub.WaitForRunT(s.T(), ctx)
	require.NoError(s.T(), mcp.WithToken(token).Signal("user_approved", ""))
	token = env.Stub.WaitForRunT(s.T(), ctx)

	activeMCP := mcp.WithToken(token)

	const n = 5
	itemIDs := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		i := i
		go func() {
			defer wg.Done()
			id, err := activeMCP.CreateItem(fmt.Sprintf("concurrent-item-%d", i))
			itemIDs[i] = id
			errs[i] = err
		}()
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(s.T(), err, "item %d failed", i)
	}

	// All 5 items should appear in ListKanbanItems.
	items := env.Client.ListKanbanItems(task.ID)
	assert.Equal(s.T(), n, len(items))

	// No duplicate IDs.
	seen := make(map[string]bool, n)
	for _, item := range items {
		assert.False(s.T(), seen[item.ID], "duplicate item ID: %s", item.ID)
		seen[item.ID] = true
	}
}
