//go:build integration

package threads

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

func TestThreads(t *testing.T) { suite.Run(t, new(ThreadsSuite)) }

type ThreadsSuite struct{ kit.IntegrationSuite }

// TestHumanOpenThreadAndAgentReply verifies the full thread lifecycle:
// human opens, agent replies, agent resolves.
func (s *ThreadsSuite) TestHumanOpenThreadAndAgentReply() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "threads-human-branch")

	// Advance to human_review.
	advanceToHumanReview(s, ctx, task.ID)

	// Subscribe to thread events.
	threadCh, closeThreads := env.WSClient.SubscribeThreads(task.ID)
	defer closeThreads()

	// Human opens a thread.
	thread := env.Client.OpenThread(task.ID, kit.OpenThreadRequest{
		File:    "main.go",
		Line:    42,
		Content: "this variable is unused",
	})
	assert.Equal(s.T(), string(domain.ReviewPhaseHumanReview), string(thread.Phase))
	assert.Equal(s.T(), "human", thread.OpenedBy)
	assert.Equal(s.T(), string(domain.ReviewThreadStatusOpen), string(thread.Status))

	// Wait for the WS event.
	waitThread(s.T(), threadCh, thread.ID, 3*time.Second)

	// Advance back to implementation so there is an active agent run.
	// (human_review → changes_requested → implementation)
	env.Client.ForceTransition(task.ID, "implementation")
	token := env.Stub.WaitForRunT(s.T(), ctx)
	mcp := env.MCPClient.WithTest(s.T()).WithToken(token)

	// Agent replies.
	require.NoError(s.T(), mcp.ReplyThread(thread.ID, "fixed in commit abc123"))

	// Resolve.
	require.NoError(s.T(), mcp.ResolveThread(thread.ID, "👍"))

	// Verify status.
	threads := env.Client.ListThreads(task.ID, "", "")
	require.Len(s.T(), threads, 1)
	assert.Equal(s.T(), string(domain.ReviewThreadStatusAgreed), string(threads[0].Status))
}

// TestForceApproveThread verifies that a human can force-approve a thread.
func (s *ThreadsSuite) TestForceApproveThread() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "threads-force-approve-branch")

	advanceToHumanReview(s, ctx, task.ID)

	thread := env.Client.OpenThread(task.ID, kit.OpenThreadRequest{
		File:    "main.go",
		Line:    1,
		Content: "style nit",
	})

	approved := env.Client.ForceApproveThread(thread.ID)
	assert.Equal(s.T(), string(domain.ReviewThreadStatusForceApproved), string(approved.Status))
}

// TestFilterThreadsByStatusAndPhase verifies that ListThreads filters work.
func (s *ThreadsSuite) TestFilterThreadsByStatusAndPhase() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "threads-filter-branch")

	advanceToAIReview(s, ctx, task.ID)

	// Open a thread via MCP (as reviewer in ai_review phase).
	token := env.Stub.WaitForRunT(s.T(), ctx)
	mcp := env.MCPClient.WithTest(s.T()).WithToken(token)

	_, err := mcp.OpenThread("main.go", 10, "logic error")
	require.NoError(s.T(), err)

	// Filter: status=open, phase=ai_review should return the thread.
	threads := env.Client.ListThreads(task.ID, "open", "ai_review")
	assert.GreaterOrEqual(s.T(), len(threads), 1)

	// Filter: status=agreed should return nothing.
	threads = env.Client.ListThreads(task.ID, "agreed", "")
	assert.Empty(s.T(), threads)
}

// advanceToHumanReview drives the task through brainstorming, spec,
// implementation, and ai_review to reach human_review. It consumes stub tokens
// along the way.
func advanceToHumanReview(
	s *ThreadsSuite,
	ctx context.Context,
	taskID string,
) {
	advanceToAIReview(s, ctx, taskID)
	token := s.Env.Stub.WaitForRunT(s.T(), ctx)
	require.NoError(s.T(), s.Env.MCPClient.WithTest(s.T()).WithToken(token).Signal("review_passed", ""))
	waitForTaskCurrentState(s.T(), s.Env, taskID, "human_review", 5*time.Second)
}

// advanceToAIReview advances to ai_review, waiting for the MCP stub at each step.
func advanceToAIReview(
	s *ThreadsSuite,
	ctx context.Context,
	taskID string,
) {
	mcp := s.Env.MCPClient.WithTest(s.T())
	token := s.Env.Stub.WaitForRunT(s.T(), ctx)
	require.NoError(s.T(), mcp.WithToken(token).Signal("user_approved", ""))
	token = s.Env.Stub.WaitForRunT(s.T(), ctx)
	require.NoError(s.T(), mcp.WithToken(token).Signal("user_approved", ""))
	token = s.Env.Stub.WaitForRunT(s.T(), ctx)
	require.NoError(s.T(), mcp.WithToken(token).Signal("implementation_complete", ""))
	waitForTaskCurrentState(s.T(), s.Env, taskID, "ai_review", 5*time.Second)
}

// waitThread blocks until a thread with the given ID is received on ch.
func waitThread(
	t *testing.T,
	ch <-chan domain.ReviewThread,
	threadID string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("waitThread: timeout waiting for thread %s", threadID)
			return
		}
		select {
		case <-time.After(remaining):
			t.Fatalf("waitThread: timeout")
			return
		case th, ok := <-ch:
			if !ok {
				return
			}
			if th.ID == threadID {
				return
			}
		}
	}
}

// waitForTaskCurrentState polls the REST endpoint until the task reaches the
// expected state or timeout elapses.
func waitForTaskCurrentState(
	t *testing.T,
	env *kit.Env,
	taskID string,
	want string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task := env.Client.GetTask(taskID)
		if task.CurrentState == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitForTaskCurrentState: timeout waiting for state %q", want)
}
