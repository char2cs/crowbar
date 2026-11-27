//go:build integration

package github_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/provider/providers/github"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Dir(file)
	for {
		if _, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
			out, _ := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
			return filepath.Clean(string(out[:len(out)-1]))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find git root")
		}
		dir = parent
	}
}

func originOf(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", "-C", dir, "remote", "get-url", "origin").Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func TestIntegration_ProtectedBranches(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed")
	}

	dir := repoRoot(t)
	p := github.New(originOf(t, dir), exec.CommandContext)

	branches, err := p.ProtectedBranches(context.Background(), dir)
	require.NoError(t, err)
	assert.IsType(t, []string{}, branches)
	t.Logf("protected branches: %v", branches)
}

func TestIntegration_PullRequestForBranch(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed")
	}

	dir := repoRoot(t)
	p := github.New(originOf(t, dir), exec.CommandContext)

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

func TestIntegration_PullRequestForBranch_NoPR(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed")
	}

	dir := repoRoot(t)
	p := github.New(originOf(t, dir), exec.CommandContext)

	pr, err := p.PullRequestForBranch(context.Background(), dir, "no-such-branch-xyzzy-12345")
	require.NoError(t, err)
	assert.Nil(t, pr)
}
