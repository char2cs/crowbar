package diff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func oscGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func oscInitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oscGitRun(t, dir, "init", "-b", "main")
	oscGitRun(t, dir, "config", "user.email", "test@test.com")
	oscGitRun(t, dir, "config", "user.name", "Test User")
	return dir
}

func oscHeadSHA(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func TestOutline_BlankContextLine_SuppressBlankEmptyConfig(t *testing.T) {
	dir := oscInitRepo(t)
	oscGitRun(t, dir, "config", "diff.suppressBlankEmpty", "true")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\n\nc\nd\n"), 0o600))
	oscGitRun(t, dir, "add", "f.txt")
	oscGitRun(t, dir, "commit", "-m", "base")
	ref := oscHeadSHA(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\n\nC\nd\n"), 0o600))

	files, err := Outline(context.Background(), dir, ref)

	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 1, OldLines: 5, NewStart: 1, NewLines: 5},
	}, files[0].Hunks)
}
