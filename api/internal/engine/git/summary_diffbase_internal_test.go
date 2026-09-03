// summary_diffbase_internal_test.go is a white-box (package git) test file for
// resolveDiffBase and looksLikeCommitSHA branches that a real git subprocess
// cannot be made to hit through the public Engine surface: a `git merge-base`
// invocation that exits zero with no output, and a base string shaped exactly
// like a commit SHA (40 chars) except for one non-hex byte.
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

// diffbaseInitRepo is a minimal stand-in for the git_test package's own
// initRepo/makeCommit/headSHA helpers, which live in package git_test and so
// are not visible from this white-box (package git) file.
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

// TestResolveDiffBase_MergeBaseSucceedsWithEmptyOutput_TreatedAsNoCandidate
// pins resolveDiffBase's defense against a `git merge-base` that exits zero
// but prints nothing: real git never does this, but the engine must not adopt
// an empty string as a resolved base commit (which would then flow into
// numstat/rev-list as a nonsensical revision) — it must simply skip the
// candidate and keep looking, ending with no base at all when nothing else
// resolves.
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

	got := e.resolveDiffBase(context.Background(), dir, "some-branch")

	assert.Empty(t, got, "an empty-but-successful merge-base must not be adopted as the resolved base")
}

// TestLooksLikeCommitSHA_FortyCharsButNonHex covers the byte-by-byte hex check:
// a base string the right LENGTH for a commit SHA (40) but carrying one byte
// outside [0-9a-fA-F] must not be classified as a SHA, or baseRefCandidates
// would skip probing it as a branch name (origin/<base> and <base>) and only
// ever try it verbatim — which cannot resolve a real branch whose name simply
// happens to be 40 characters long.
func TestLooksLikeCommitSHA_FortyCharsButNonHex(t *testing.T) {
	fortyCharsOneBadByte := strings.Repeat("a", 39) + "g"
	require.Len(t, fortyCharsOneBadByte, 40)

	assert.False(t, looksLikeCommitSHA(fortyCharsOneBadByte))
}

// TestBaseRefCandidates_NonHexFortyCharString_TreatedAsBranchName is the
// behavioral pin for the above: baseRefCandidates must probe it as a branch
// (origin/<base> and <base>), not treat it as a literal SHA.
func TestBaseRefCandidates_NonHexFortyCharString_TreatedAsBranchName(t *testing.T) {
	name := strings.Repeat("a", 39) + "g"
	require.Len(t, name, 40)

	got := baseRefCandidates(name)

	require.Len(t, got, 2)
	assert.Equal(t, "origin/"+name, got[0])
	assert.Equal(t, name, got[1])
}
