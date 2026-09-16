package v0

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
)

// initCleanGitRepo builds a real, committed git repo with no uncommitted
// changes — the one shape that makes `git status` report a nil Files slice,
// which is what appendGitStatus's normalize branch needs to see. The other
// gitSnapshot tests in this package use a bare t.TempDir() as WorktreePath,
// which is not a git repo at all and so only ever exercises Status's error
// path, never this one.
func initCleanGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	run("commit", "--allow-empty", "-m", "init")
	return dir
}

// TestGitSnapshot_CleanWorkspace_NormalizesNilFilesToEmptyArray covers
// appendGitStatus's normalize branch: a clean tree's GitStatus carries a nil
// Files slice, which must come back as [] on the wire (matching the REST DTO
// contract), never null.
//
// The scope is HIERARCHICAL because that is the only shape that still resolves
// to a workspace: a bare id is a chat id now (see gitSnapshot's doc comment and
// TestGitSnapshot_BareScopeIsNotReadAsAWorkspaceID), so naming "w1" alone would
// replay nothing and assert the normalize branch vacuously.
func TestGitSnapshot_CleanWorkspace_NormalizesNilFilesToEmptyArray(t *testing.T) {
	a := newAppForSnapshot(t)
	repo := initCleanGitRepo(t)
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspacerepo.CreateInput{
			ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "main", WorktreePath: repo,
		},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
	// The workspace store projection is async (Send, not SendWait): drain it so
	// scopedWorkspaceRows' Get sees the row.
	a.Repositories.WaitQuiescent()

	got := gitSnapshot(a)("p1/r1/w1")

	require.Len(t, got, 1)
	assert.Equal(t, "main", got[0].Status.Branch)
	assert.NotNil(t, got[0].Status.Files, "a clean tree's nil Files must be normalized to a non-nil empty slice")
	assert.Empty(t, got[0].Status.Files)
}
