//go:build integration

package github_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/provider/providers/github"
)

// repoRoot walks up from this file to find the git root so the test works
// regardless of where it is run from.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Dir(file)
	for {
		if _, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
			out, _ := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
			return filepath.Clean(string(out[:len(out)-1])) // trim trailing newline
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find git root")
		}
		dir = parent
	}
}

// TestIntegration_ProtectedBranches calls the real gh CLI against this repo.
func TestIntegration_ProtectedBranches(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed")
	}

	dir := repoRoot(t)
	p := github.New()

	branches, err := p.ProtectedBranches(context.Background(), dir)
	require.NoError(t, err)
	// Result may be empty if no branches are protected — that is valid.
	// What matters is no error and the type is correct.
	assert.IsType(t, []string{}, branches)
	t.Logf("protected branches: %v", branches)
}

// TestIntegration_PullRequestForBranch exercises the real `gh pr list`
// invocation + parsing. It does NOT pin a specific live PR: the original fixture
// (PR #10 open on feature/wave2-engines) drifted when that PR merged, so the test
// now asserts the call succeeds and that any returned PR is well-typed — accepting
// "no open PR" as valid.
func TestIntegration_PullRequestForBranch(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed")
	}

	dir := repoRoot(t)
	p := github.New()

	pr, err := p.PullRequestForBranch(context.Background(), dir, "feature/wave2-engines")
	require.NoError(t, err)

	if pr == nil {
		t.Log("no open PR for feature/wave2-engines (the original PR #10 merged) — call succeeded")
		return
	}
	assert.Positive(t, pr.Number)
	assert.NotEmpty(t, pr.Status)
	assert.NotEmpty(t, pr.URL)
	assert.NotEmpty(t, pr.Title)
	t.Logf("PR: #%d %q status=%s target=%s", pr.Number, pr.Title, pr.Status, pr.TargetBranch)
}

// TestIntegration_PullRequestForBranch_NoPR verifies nil is returned for a branch with no PR.
func TestIntegration_PullRequestForBranch_NoPR(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed")
	}

	dir := repoRoot(t)
	p := github.New()

	// Use a branch name that is extremely unlikely to have a PR.
	pr, err := p.PullRequestForBranch(context.Background(), dir, "no-such-branch-xyzzy-12345")
	require.NoError(t, err)
	assert.Nil(t, pr)
}
