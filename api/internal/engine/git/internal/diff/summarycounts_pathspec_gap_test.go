package diff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func summaryGapGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func summaryGapInitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	summaryGapGitRun(t, dir, "init", "-b", "main")
	summaryGapGitRun(t, dir, "config", "user.email", "test@test.com")
	summaryGapGitRun(t, dir, "config", "user.name", "Test User")
	return dir
}

func summaryGapCommit(t *testing.T, dir, name, content, message string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	summaryGapGitRun(t, dir, "add", name)
	summaryGapGitRun(t, dir, "commit", "-m", message)
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func TestSummaryCounts_PathspecReDiffFails_PropagatesError(t *testing.T) {
	dir := summaryGapInitRepo(t)
	ref := summaryGapCommit(t, dir, "a.txt", "1\n", "c1")
	summaryGapCommit(t, dir, "a.txt", "1\n2\n", "c2")

	entries := []gitdomain.ReviewFileSummary{
		{Path: "a.txt", Status: gitdomain.GitFileStatusModified},
	}
	dirty := []string{"a.txt"}

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "index"), []byte("garbage"), 0o600))

	_, err := summaryCounts(context.Background(), dir, ref, entries, dirty)

	require.Error(t, err)
	require.ErrorContains(t, err, "dirty numstat")
}
