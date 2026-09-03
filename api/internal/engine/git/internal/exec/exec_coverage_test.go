package exec_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	"github.com/char2cs/crowbar/api/internal/perf"
)

// TestGitWithEnv_InjectsExtraEnvironmentVariable is GitWithEnv's only test:
// it has no production callers today, but it is exported package API, so its
// one job — appending extraEnv to the subprocess environment rather than
// replacing it — needs direct coverage. `git var GIT_AUTHOR_IDENT` echoes back
// whatever GIT_AUTHOR_NAME the subprocess environment carries, which proves
// the override actually reached the child process.
func TestGitWithEnv_InjectsExtraEnvironmentVariable(t *testing.T) {
	dir := initRepo(t)

	r := exec.GitWithEnv(context.Background(), dir, []string{
		"GIT_AUTHOR_NAME=Injected Author",
		"GIT_AUTHOR_EMAIL=injected@example.com",
	}, "var", "GIT_AUTHOR_IDENT")

	require.Equal(t, 0, r.ExitCode, r.Stderr)
	assert.Contains(t, r.Stdout, "Injected Author")
	assert.Contains(t, r.Stdout, "injected@example.com")
}

// TestGit_SubcommandNameSkipsNonDashCFlags extends
// TestGit_SubcommandNameIgnoresFlags (which only covers the "-c" skip) to a
// different leading flag, proving the general HasPrefix("-") skip branch also
// works and not just the special-cased "-c value" pair.
func TestGit_SubcommandNameSkipsNonDashCFlags(t *testing.T) {
	dir := initRepo(t)
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	_ = exec.Git(context.Background(), dir, "--no-pager", "status")

	var names []string
	for _, s := range perf.Snapshot() {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "git.status")
}

// TestGit_SubcommandNameAllFlagsFallsBackToUnknown covers subcommandName's
// final fallback: an argument list containing nothing but flags never finds a
// subcommand name to return.
func TestGit_SubcommandNameAllFlagsFallsBackToUnknown(t *testing.T) {
	dir := initRepo(t)
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	_ = exec.Git(context.Background(), dir, "--no-pager")

	var names []string
	for _, s := range perf.Snapshot() {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "git.unknown")
}

// TestClassifyTimeout_NoParentDeadline_ReportsDefaultTimeout covers the one
// classifyTimeout branch a real subprocess cannot reach in test time: a
// caller-supplied context with no deadline of its own (so classifyTimeout's
// parentErr is nil) that still got killed by the internal 60s GitOpTimeout.
func TestClassifyTimeout_NoParentDeadline_ReportsDefaultTimeout(t *testing.T) {
	r := exec.ClassifyTimeout(context.DeadlineExceeded, nil, exec.Result{ExitCode: -1})

	assert.True(t, strings.HasPrefix(r.Stderr, "git operation timed out after "))
	assert.Contains(t, r.Stderr, exec.GitOpTimeout.String())
}
