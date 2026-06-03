//go:build integration

package crash

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

func TestCrash(t *testing.T) { suite.Run(t, new(CrashSuite)) }

// CrashSuite does NOT embed IntegrationSuite because it manages two Env
// instances manually against the same shared homeDir.
type CrashSuite struct {
	suite.Suite
}

// TestOrphanedRunRecovery starts a task in env1, crashes without signalling,
// then restarts with env2 pointing at the same home dir and asserts recovery.
func (s *CrashSuite) TestOrphanedRunRecovery() {
	homeDir := s.T().TempDir()

	// --- env1: start a task and confirm the agent run is running ---
	env1 := kit.NewEnvWithHome(s.T(), homeDir)

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env1.Client)
	repo := kit.DefaultRepository(env1.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env1.Client, repo.ID, "crash-recovery-branch")

	ctx := context.Background()
	_ = env1.Stub.WaitForRunT(s.T(), ctx) // confirms AgentRun is running

	runs := env1.Client.ListAgentRuns(task.ID)
	require.Len(s.T(), runs, 1)
	assert.Equal(s.T(), string(domain.AgentRunStatusRunning), string(runs[0].Status))

	// Crash: close env1 without letting the stub signal — orphaned run.
	env1.Close()

	// Brief pause to let the SQLite write complete.
	time.Sleep(100 * time.Millisecond)

	// --- env2: restart against the same home dir ---
	env2 := kit.NewEnvWithHome(s.T(), homeDir)
	defer env2.Close()

	// The orphaned run should be failed after recovery.
	waitForRunStatus(s.T(), env2, task.ID, domain.AgentRunStatusFailed, 5*time.Second)

	// The task should be paused.
	taskAfter := env2.Client.GetTask(task.ID)
	assert.Equal(s.T(), string(domain.TaskStatusPaused), string(taskAfter.Status))

	// Retry creates a new AgentRun.
	env2.Client.RetryTask(task.ID)
	_ = env2.Stub.WaitForRunT(s.T(), ctx) // new run started

	runsAfter := env2.Client.ListAgentRuns(task.ID)
	require.GreaterOrEqual(s.T(), len(runsAfter), 2)

	running := false
	for _, r := range runsAfter {
		if r.Status == domain.AgentRunStatusRunning {
			running = true
		}
	}
	assert.True(s.T(), running, "expected a new running agent run after retry")
}

// waitForRunStatus polls until at least one run for taskID has the given status.
func waitForRunStatus(
	t *testing.T,
	env *kit.Env,
	taskID string,
	want domain.AgentRunStatus,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		runs := env.Client.ListAgentRuns(taskID)
		for _, r := range runs {
			if r.Status == want {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("waitForRunStatus: timeout waiting for status %q", want)
}
