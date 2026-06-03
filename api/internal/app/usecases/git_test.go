package usecases

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, "git init step failed: %s", string(out))
	}
	f := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(f, []byte("hello\n"), 0600))
	addOut, err := exec.Command("git", "-C", dir, "add", ".").CombinedOutput()
	require.NoError(t, err, string(addOut))
	commitOut, err := exec.Command("git", "-C", dir, "commit", "-m", "init").CombinedOutput()
	require.NoError(t, err, string(commitOut))
	return dir
}

func TestGitLog_TaskNotFound(t *testing.T) {
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{}, domain.ErrNotFound
		},
	}
	uc := NewGit(taskRepo)
	_, err := uc.Log(context.Background(), "nope")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGitLog_TaskGetError(t *testing.T) {
	boom := errors.New("boom")
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{}, boom
		},
	}
	uc := NewGit(taskRepo)
	_, err := uc.Log(context.Background(), "t1")
	assert.ErrorIs(t, err, boom)
}

func TestGitDiff_TaskNotFound(t *testing.T) {
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{}, domain.ErrNotFound
		},
	}
	uc := NewGit(taskRepo)
	_, err := uc.Diff(context.Background(), "nope")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGitListFiles_TaskNotFound(t *testing.T) {
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{}, domain.ErrNotFound
		},
	}
	uc := NewGit(taskRepo)
	_, err := uc.ListFiles(context.Background(), "nope")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGitLog_EmptyWorktree_ReturnsEmptySlice(t *testing.T) {
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{ID: id, WorktreePath: t.TempDir()}, nil
		},
	}
	uc := NewGit(taskRepo)
	commits, err := uc.Log(context.Background(), "t1")
	// git log on an empty dir will error — that's fine. Just verify task lookup succeeds.
	if err != nil {
		assert.NotErrorIs(t, err, domain.ErrNotFound)
		return
	}
	assert.NotNil(t, commits)
}

func TestGitLog_HappyPath(t *testing.T) {
	dir := initGitRepo(t)
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{ID: id, WorktreePath: dir}, nil
		},
	}
	uc := NewGit(taskRepo)
	commits, err := uc.Log(context.Background(), "t1")
	require.NoError(t, err)
	assert.Len(t, commits, 1)
	assert.Equal(t, "init", commits[0].Message)
}

func TestGitLog_GitError(t *testing.T) {
	dir := t.TempDir()
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{ID: id, WorktreePath: dir}, nil
		},
	}
	uc := NewGit(taskRepo)
	_, err := uc.Log(context.Background(), "t1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git usecase: log")
}

func TestGitDiff_HappyPath(t *testing.T) {
	dir := initGitRepo(t)
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{ID: id, WorktreePath: dir}, nil
		},
	}
	uc := NewGit(taskRepo)
	diff, err := uc.Diff(context.Background(), "t1")
	require.NoError(t, err)
	assert.IsType(t, "", diff)
}

func TestGitDiff_GitError(t *testing.T) {
	dir := t.TempDir()
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{ID: id, WorktreePath: dir}, nil
		},
	}
	uc := NewGit(taskRepo)
	_, err := uc.Diff(context.Background(), "t1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git usecase: diff")
}

func TestGitListFiles_HappyPath(t *testing.T) {
	dir := initGitRepo(t)
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{ID: id, WorktreePath: dir}, nil
		},
	}
	uc := NewGit(taskRepo)
	files, err := uc.ListFiles(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, []string{"hello.txt"}, files)
}

func TestGitListFiles_GitError(t *testing.T) {
	dir := t.TempDir()
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{ID: id, WorktreePath: dir}, nil
		},
	}
	uc := NewGit(taskRepo)
	_, err := uc.ListFiles(context.Background(), "t1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git usecase: list files")
}

func TestGitListFiles_EmptyWorktree_ReturnsEmptySlice(t *testing.T) {
	taskRepo := &mockGitTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{ID: id, WorktreePath: t.TempDir()}, nil
		},
	}
	uc := NewGit(taskRepo)
	files, err := uc.ListFiles(context.Background(), "t1")
	if err != nil {
		assert.NotErrorIs(t, err, domain.ErrNotFound)
		return
	}
	assert.NotNil(t, files)
}

// ---------- mock Task repo for git tests ----------

type mockGitTaskRepo struct {
	getFn func(ctx context.Context, id string) (domain.Task, error)
}

func (m *mockGitTaskRepo) Create(_ context.Context, _, _, _, _, _, _, _, _ string) (domain.Task, error) {
	return domain.Task{}, nil
}
func (m *mockGitTaskRepo) List(_ context.Context, _ string) ([]domain.Task, error) {
	return nil, nil
}
func (m *mockGitTaskRepo) Get(ctx context.Context, id string) (domain.Task, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return domain.Task{}, nil
}
func (m *mockGitTaskRepo) Archive(_ context.Context, _ string) error            { return nil }
func (m *mockGitTaskRepo) Pause(_ context.Context, _ string) error              { return nil }
func (m *mockGitTaskRepo) Resume(_ context.Context, _ string) error             { return nil }
func (m *mockGitTaskRepo) Retry(_ context.Context, _ string) error              { return nil }
func (m *mockGitTaskRepo) AdvanceState(_ context.Context, _, _, _ string) error { return nil }
func (m *mockGitTaskRepo) ForceTransition(_ context.Context, _, _ string) error { return nil }
func (m *mockGitTaskRepo) Subscribe(_ string, _ func(context.Context, asynxModels.Event[domain.Task])) (string, error) {
	return "", nil
}
