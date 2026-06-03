//go:build integration

package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestLifecycle(t *testing.T) { suite.Run(t, new(LifecycleSuite)) }

type LifecycleSuite struct{ kit.IntegrationSuite }

// TestFullTaskLifecycle drives a Task through all states of the builtin
// feature-development flow, asserting WebSocket events for each transition.
func (s *LifecycleSuite) TestFullTaskLifecycle() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "feature-full-lifecycle")

	taskCh, closeTask := env.WSClient.SubscribeTasks(task.ID)
	defer closeTask()

	// Drain the initial sync frame.
	waitTaskState(s.T(), taskCh, task.CurrentState, 2*time.Second)

	mcp := env.MCPClient.WithTest(s.T())

	// brainstorming → spec
	token := env.Stub.WaitForRunT(s.T(), ctx)
	assert.NoError(s.T(), mcp.WithToken(token).Signal("user_approved", "brainstorming done"))
	waitTaskState(s.T(), taskCh, "spec", 3*time.Second)
	assert.Equal(s.T(), "spec", env.Client.GetTask(task.ID).CurrentState)

	// spec → implementation
	token = env.Stub.WaitForRunT(s.T(), ctx)
	assert.NoError(s.T(), mcp.WithToken(token).Signal("user_approved", "spec complete"))
	waitTaskState(s.T(), taskCh, "implementation", 3*time.Second)

	// implementation → ai_review
	token = env.Stub.WaitForRunT(s.T(), ctx)
	assert.NoError(s.T(), mcp.WithToken(token).Signal("implementation_complete", "all tasks done"))
	waitTaskState(s.T(), taskCh, "ai_review", 3*time.Second)

	// ai_review → human_review
	token = env.Stub.WaitForRunT(s.T(), ctx)
	assert.NoError(s.T(), mcp.WithToken(token).Signal("review_passed", "review passed"))
	waitTaskState(s.T(), taskCh, "human_review", 3*time.Second)

	// human_review is a human-only state; force-transition to complete.
	final := env.Client.ForceTransition(task.ID, "complete")
	assert.Equal(s.T(), "complete", final.CurrentState)

	// At least 4 completed agent runs (brainstorming, spec, implementation, ai_review).
	runs := env.Client.ListAgentRuns(task.ID)
	assert.GreaterOrEqual(s.T(), len(runs), 4)
}

// waitTaskState blocks until a task with the given current_state arrives on ch
// or the timeout elapses.
func waitTaskState(
	t *testing.T,
	ch <-chan domain.Task,
	want string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("waitTaskState: timeout waiting for state %q", want)
			return
		}
		select {
		case <-time.After(remaining):
			t.Fatalf("waitTaskState: timeout waiting for state %q", want)
			return
		case task, ok := <-ch:
			if !ok {
				t.Fatalf("waitTaskState: channel closed before state %q", want)
				return
			}
			if task.CurrentState == want {
				return
			}
		}
	}
}
