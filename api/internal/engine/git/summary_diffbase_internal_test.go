package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func diffbaseInitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	diffbaseGitRun(t, dir, "init", "-b", "main")
	diffbaseGitRun(t, dir, "config", "user.email", "test@test.com")
	diffbaseGitRun(t, dir, "config", "user.name", "Test User")
	return dir
}

func diffbaseGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func diffbaseCommit(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	diffbaseGitRun(t, dir, "add", name)
	diffbaseGitRun(t, dir, "commit", "-m", name)
}

func TestResolveDiffBase_MergeBaseSucceedsWithEmptyOutput_TreatedAsNoCandidate(t *testing.T) {
	dir := diffbaseInitRepo(t)
	diffbaseCommit(t, dir, "f.txt", "content\n")

	fake := func(ctx context.Context, repoDir string, args ...string) gitexec.Result {
		if len(args) > 0 && args[0] == "merge-base" {
			return gitexec.Result{ExitCode: 0, Stdout: ""}
		}
		return gitexec.Git(ctx, repoDir, args...)
	}
	e := &engine{exec: fake}

	got, _ := e.resolveDiffBase(context.Background(), dir, "some-branch")

	assert.Empty(t, got, "an empty-but-successful merge-base must not be adopted as the resolved base")
}

func TestLooksLikeCommitSHA_FortyCharsButNonHex(t *testing.T) {
	fortyCharsOneBadByte := strings.Repeat("a", 39) + "g"
	require.Len(t, fortyCharsOneBadByte, 40)

	assert.False(t, looksLikeCommitSHA(fortyCharsOneBadByte))
}

func TestBaseRefCandidates_NonHexFortyCharString_TreatedAsBranchName(t *testing.T) {
	name := strings.Repeat("a", 39) + "g"
	require.Len(t, name, 40)

	got := baseRefCandidates(name)

	require.Len(t, got, 2)
	assert.Equal(t, "origin/"+name, got[0])
	assert.Equal(t, name, got[1])
}

func TestComputeWorkingTreeSummary_BranchBase_SpawnBudget(t *testing.T) {
	dir := diffbaseInitRepo(t)
	diffbaseCommit(t, dir, "base.txt", "base\n")
	diffbaseGitRun(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	diffbaseGitRun(t, dir, "checkout", "-b", "feature")
	diffbaseCommit(t, dir, "feature.txt", "feature\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature\nmore\n"), 0o600))

	var spawns []string
	counting := func(ctx context.Context, repoDir string, args ...string) gitexec.Result {
		spawns = append(spawns, strings.Join(args, " "))
		return gitexec.Git(ctx, repoDir, args...)
	}
	e := &engine{exec: counting}

	added, deleted, hasConflicts, hasCommits, err := e.computeWorkingTreeSummary(context.Background(), dir, "main")

	require.NoError(t, err)
	assert.Equal(t, 2, added, "committed + uncommitted lines against the merge-base")
	assert.Equal(t, 0, deleted)
	assert.False(t, hasConflicts)
	assert.True(t, hasCommits)
	assert.LessOrEqual(t, len(spawns), 4, "git spawns per summary (conflict check runs outside e.exec): %v", spawns)
}
