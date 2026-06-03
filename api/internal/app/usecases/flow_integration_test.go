package usecases_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	appHub "github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// TestFeatureDevelopmentFlow_Integration exercises the full default
// feature-development flow using real SQLite and real git. It verifies:
// - task creation starts in "brainstorming"
// - list returns the created task
// - invalid state is rejected
// - force-transition through every state in the builtin flow succeeds
// - archive marks the task archived
func TestFeatureDevelopmentFlow_Integration(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	ad, err := adapter.New(adapter.WithHomeDir(tmpDir))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ad.Close() })

	eng, err := engine.New(ctx, nil, engine.WithHomeDir(tmpDir))
	require.NoError(t, err)

	chatHub := appHub.NewChatHub()

	repos, err := repositories.New(ctx, eng, ad, chatHub)
	require.NoError(t, err)

	ucs := usecases.New(repos, eng.FlowLoader)

	// Set up a real git repository so git worktree add can succeed.
	repoDir := filepath.Join(tmpDir, "myrepo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	gitInit(t, repoDir)
	// Pre-create the worktrees parent so git has a place to put it.
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".worktrees"), 0o755))

	// Create project + repository pointing at the real git repo.
	project, err := ucs.Project.Create(ctx, "Test Project")
	require.NoError(t, err)

	repo, err := ucs.Repository.Create(ctx, project.ID, "myrepo", repoDir)
	require.NoError(t, err)

	// Create task — should land in "brainstorming" (first state of builtin flow).
	task, err := ucs.Task.Create(ctx, repo.ID, "Add feature X", "feat-x", "main", "")
	require.NoError(t, err)
	assert.Equal(t, "brainstorming", task.CurrentState)
	assert.Equal(t, domain.TaskStatusPending, task.Status)

	// List tasks for this repository — must contain exactly the one we created.
	tasks, err := ucs.Task.List(ctx, repo.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, task.ID, tasks[0].ID)

	// Invalid state must be rejected.
	err = ucs.Task.ForceTransition(ctx, task.ID, "nonexistent_state")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent_state")

	// Force-transition through every state in the builtin flow in order.
	states := []string{
		"spec",
		"implementation",
		"ai_review",
		"human_review",
		"complete",
	}
	for _, state := range states {
		require.NoError(t, ucs.Task.ForceTransition(ctx, task.ID, state), "transition to %s", state)
		got, err := ucs.Task.Get(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, state, got.CurrentState, "after transition to %s", state)
	}

	// Archive and verify status.
	require.NoError(t, ucs.Task.Archive(ctx, task.ID))
	archived, err := ucs.Task.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusArchived, archived.Status)
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s failed: %s", args[0], out)
	}
	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	run("git", "commit", "--allow-empty", "-m", "init")
}
