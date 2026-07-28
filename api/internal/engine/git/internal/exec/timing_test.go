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
