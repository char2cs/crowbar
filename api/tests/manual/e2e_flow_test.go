//go:build manual

package manual

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullFlowWithRealAgent drives a task through all five states of the
// feature-development flow using a real claude subprocess.
//
// Run with:
//
//	go test -v -tags=manual -timeout=30m -run TestFullFlowWithRealAgent ./tests/manual/
func TestFullFlowWithRealAgent(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not found in PATH; skipping manual e2e test")
	}
	if !claudeAuthenticated(t) {
		t.Skip("claude is not authenticated (no ANTHROPIC_API_KEY and no usable " +
			"subscription credentials); skipping manual e2e test")
	}

	env := kit.NewRealEnv(t)

	// Preserve agent run artifacts (events.jsonl, diff.patch) for diagnosis.
	// t.TempDir is removed after the test, so copy runs/ out before that.
	t.Cleanup(func() {
		runsDir := filepath.Join(env.HomeDir, "runs")
		dst := "/tmp/e2e-runs"
		_ = os.RemoveAll(dst)
		if err := copyDir(runsDir, dst); err != nil {
			t.Logf("could not preserve runs dir: %v", err)
			return
		}
		t.Logf("agent run artifacts preserved at %s", dst)
	})

	gitRepo := kit.NewGitRepo(t)

	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := env.Client.CreateTask(repo.ID, kit.CreateTaskRequest{
		Title:      "Add a hello.go file with a Hello() function and a test for it",
		BranchName: "feature/hello",
		BaseBranch: "main",
		FlowPath:   "",
	})
	t.Logf("task created: id=%s state=%s worktree=%s", task.ID, task.CurrentState, task.WorktreePath)

	taskCh, closeTask := env.WSClient.SubscribeTasks(task.ID)
	defer closeTask()

	const stateTimeout = 3 * time.Minute

	t.Log("waiting for agent to complete brainstorming…")
	waitManualTaskState(t, taskCh, "spec", stateTimeout)
	t.Log("✓ spec")

	t.Log("waiting for agent to complete spec…")
	waitManualTaskState(t, taskCh, "implementation", stateTimeout)
	t.Log("✓ implementation")

	t.Log("waiting for agent to complete implementation…")
	waitManualTaskState(t, taskCh, "ai_review", stateTimeout)
	t.Log("✓ ai_review")

	t.Log("waiting for agent to complete ai_review…")
	waitManualTaskState(t, taskCh, "human_review", stateTimeout)
	t.Log("✓ human_review")

	t.Log("force-transitioning to complete…")
	final := env.Client.ForceTransition(task.ID, "complete")
	require.Equal(t, "complete", final.CurrentState)
	t.Log("✓ complete")

	// hello.go must exist in the worktree after implementation.
	// Fetch fresh task to get the latest WorktreePath.
	freshTask := env.Client.GetTask(task.ID)
	helloPath := filepath.Join(freshTask.WorktreePath, "hello.go")
	_, statErr := os.Stat(helloPath)
	assert.NoError(t, statErr, "hello.go should exist in worktree at %s", helloPath)

	// At least 4 agent runs (brainstorming, spec, implementation, ai_review).
	runs := env.Client.ListAgentRuns(task.ID)
	assert.GreaterOrEqual(t, len(runs), 4, "expected ≥4 agent runs, got %d", len(runs))

	// No run should be failed.
	for _, r := range runs {
		assert.NotEqual(t, domain.AgentRunStatusFailed, r.Status,
			"agent run %s should not be failed", r.ID)
	}
}

// copyDir recursively copies the directory tree rooted at src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// claudeAuthenticated reports whether the claude CLI can authenticate. It is
// satisfied either by ANTHROPIC_API_KEY being set or by a quick non-interactive
// probe succeeding (covering subscription/OAuth credentials).
func claudeAuthenticated(t *testing.T) bool {
	t.Helper()
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", "-p", "say OK", "--output-format", "text")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("claude auth probe failed: %v: %s", err, string(out))
		return false
	}
	return len(out) > 0
}

// waitManualTaskState blocks until a Task with CurrentState == want arrives on ch
// or the timeout elapses.
func waitManualTaskState(
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
			t.Fatalf("timeout waiting for task state %q", want)
			return
		}
		select {
		case <-time.After(remaining):
			t.Fatalf("timeout waiting for task state %q", want)
			return
		case task, ok := <-ch:
			if !ok {
				t.Fatalf("WS channel closed before state %q", want)
				return
			}
			t.Logf("  state update: %s", task.CurrentState)
			if task.CurrentState == want {
				return
			}
		}
	}
}
