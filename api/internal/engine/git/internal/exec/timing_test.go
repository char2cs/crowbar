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

func TestGit_RecordsSampleNamedForSubcommand(t *testing.T) {
	dir := initRepo(t)
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	r := exec.Git(context.Background(), dir, "status")
	require.Equal(t, 0, r.ExitCode)

	var names []string
	for _, s := range perf.Snapshot() {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "git.status")
}

func TestGit_RecordsNothingWhenDisabled(t *testing.T) {
	dir := initRepo(t)
	perf.Reset()
	perf.SetEnabled(false)

	_ = exec.Git(context.Background(), dir, "status")

	assert.Empty(t, perf.Snapshot())
}

func TestGit_SubcommandNameIgnoresFlags(t *testing.T) {
	dir := initRepo(t)
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	_ = exec.Git(context.Background(), dir, "-c", "core.quotepath=false", "status")

	for _, s := range perf.Snapshot() {
		assert.False(t, strings.Contains(s.Name, "-c"), "sample name leaked a flag: %s", s.Name)
	}
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
