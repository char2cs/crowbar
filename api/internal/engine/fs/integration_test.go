//go:build integration

package fs_test

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"
)

func gitRun(
	t *testing.T,
	dir string,
	args ...string,
) {
	t.Helper()
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func gitOutput(
	t *testing.T,
	dir string,
	args ...string,
) string {
	t.Helper()
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "Test User")
	return dir
}

func makeCommit(
	t *testing.T,
	dir string,
	filename string,
	content string,
	message string,
) {
	t.Helper()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	gitRun(t, dir, "add", filename)
	gitRun(t, dir, "commit", "-m", message)
}

func headSHA(
	t *testing.T,
	dir string,
) string {
	t.Helper()
	return gitOutput(t, dir, "rev-parse", "HEAD")
}

// TestGate1_StatusDiffCommitHunkStage verifies the core git read + write path.
func TestGate1_StatusDiffCommitHunkStage(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	git := enginegit.New()

	makeCommit(t, dir, "hello.go", "package main\n\nfunc Hello() string {\n\treturn \"hello\"\n}\n", "init")

	status, err := git.Status(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, "main", status.Branch)
	assert.Empty(t, status.Files)

	modPath := filepath.Join(dir, "hello.go")
	require.NoError(t, os.WriteFile(modPath, []byte(
		"package main\n\nfunc Hello() string {\n\treturn \"hello world\"\n}\n\nfunc Bye() string {\n\treturn \"bye\"\n}\n",
	), 0600))

	status, err = git.Status(ctx, dir)
	require.NoError(t, err)
	require.NotEmpty(t, status.Files)
	assert.Equal(t, domain.GitFileStatusModified, status.Files[0].Status)
	assert.False(t, status.Files[0].Staged)

	files, err := git.Diff(ctx, dir, false)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.NotEmpty(t, files[0].Hunks)

	hunkID := files[0].Hunks[0].HunkID
	assert.NotEmpty(t, hunkID)

	err = git.StageHunk(ctx, dir, "hello.go", hunkID)
	require.NoError(t, err)

	status, err = git.Status(ctx, dir)
	require.NoError(t, err)
	hasStaged := false
	for _, f := range status.Files {
		if f.Staged {
			hasStaged = true
		}
	}
	assert.True(t, hasStaged)

	err = git.Commit(ctx, dir, "Add changes", "")
	require.NoError(t, err)

	status, err = git.Status(ctx, dir)
	require.NoError(t, err)
	for _, f := range status.Files {
		assert.False(t, f.Staged)
	}
}

// TestGate1_WatcherFiresFanOut verifies watcher → fan-out → SyncWorkingTreeState.
func TestGate1_WatcherFiresFanOut(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	git := enginegit.New()

	makeCommit(t, dir, "seed.txt", "seed\n", "seed commit")
	fork := headSHA(t, dir)

	var mu sync.Mutex
	var fileEvents []domain.FileChangeEvent

	d := &captureDispatcher{
		onFile: func(evt domain.FileChangeEvent) {
			mu.Lock()
			defer mu.Unlock()
			fileEvents = append(fileEvents, evt)
		},
	}

	gitProv := &gitProvider{engine: git}
	w := watch.NewWatcher("ws-gate1", dir, fork, gitProv, d)
	require.NoError(t, w.Start(ctx))
	t.Cleanup(w.Stop)
	time.Sleep(50 * time.Millisecond)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("new content\n"), 0600))

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(fileEvents)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, fileEvents)
	assert.Equal(t, "ws-gate1", fileEvents[0].WsID)
}

// TestGate1_GitStatusPushOnMutation verifies GitStatus is recomputed after mutations.
func TestGate1_GitStatusPushOnMutation(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	git := enginegit.New()

	makeCommit(t, dir, "base.txt", "base\n", "base")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "modified.txt"), []byte("content\n"), 0600))

	status, err := git.Status(ctx, dir)
	require.NoError(t, err)
	require.Len(t, status.Files, 1)
	assert.Equal(t, domain.GitFileStatusUntracked, status.Files[0].Status)

	err = git.StageFile(ctx, dir, "modified.txt")
	require.NoError(t, err)

	status, err = git.Status(ctx, dir)
	require.NoError(t, err)
	require.Len(t, status.Files, 1)
	assert.Equal(t, domain.GitFileStatusAdded, status.Files[0].Status)
	assert.True(t, status.Files[0].Staged)
}

// TestGate1_ConflictResolutionLifecycle verifies conflict detection → resolve → continue.
func TestGate1_ConflictResolutionLifecycle(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	git := enginegit.New()

	makeCommit(t, dir, "conflict.txt", "line1\nline2\nline3\n", "base commit")

	gitRun(t, dir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nFEATURE\nline3\n"), 0600))
	gitRun(t, dir, "add", "conflict.txt")
	gitRun(t, dir, "commit", "-m", "feature change")

	gitRun(t, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nMAIN\nline3\n"), 0600))
	gitRun(t, dir, "add", "conflict.txt")
	gitRun(t, dir, "commit", "-m", "main change")

	cmd := osexec.Command("git", "merge", "feature")
	cmd.Dir = dir
	_ = cmd.Run()

	conflicted, err := git.ConflictedFiles(ctx, dir)
	require.NoError(t, err)
	require.Contains(t, conflicted, "conflict.txt")

	hunks, err := git.ConflictHunks(ctx, dir, "conflict.txt")
	require.NoError(t, err)
	require.NotEmpty(t, hunks)
	assert.Equal(t, domain.ConflictResolutionUnresolved, hunks[0].Resolution)

	err = git.ResolveHunk(ctx, dir, "conflict.txt", hunks[0].ID, domain.ConflictResolutionOurs, "")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "conflict.txt"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "<<<<<<<")

	err = git.OperationContinue(ctx, dir)
	require.NoError(t, err)

	status, err := git.Status(ctx, dir)
	require.NoError(t, err)
	for _, f := range status.Files {
		assert.NotEqual(t, domain.GitFileStatusConflicted, f.Status)
	}
}

type captureDispatcher struct {
	onFile func(domain.FileChangeEvent)
	onSync func(watch.SyncInput)
}

func (d *captureDispatcher) OnFileChange(
	_ context.Context,
	evt domain.FileChangeEvent,
) {
	if d.onFile != nil {
		d.onFile(evt)
	}
}

func (d *captureDispatcher) OnGitStatus(
	_ context.Context,
	_ string,
	_ domain.GitStatus,
) {}

func (d *captureDispatcher) OnSyncWorkingTreeState(
	_ context.Context,
	input watch.SyncInput,
) {
	if d.onSync != nil {
		d.onSync(input)
	}
}

type gitProvider struct {
	engine enginegit.Engine
}

func (g *gitProvider) ComputeStatus(
	ctx context.Context,
	repoPath string,
) (domain.GitStatus, error) {
	return g.engine.Status(ctx, repoPath)
}

func (g *gitProvider) ComputeWorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	return g.engine.WorkingTreeSummary(ctx, repoPath, forkPointSha)
}
