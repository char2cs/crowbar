// Package diff (internal test) exercises an outlineScan branch that a
// default-configured real git repo cannot reliably reproduce.
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

// TestOutline_BlankContextLine_SuppressBlankEmptyConfig pins body()'s
// len(line)==0 branch for real: a blank source line is rendered by git with a
// single leading space (" ") by default, which is NOT a zero-length line and
// does not exercise this branch at all. Only with `diff.suppressBlankEmpty`
// set — a real, user-controllable git config this daemon does not itself set,
// so a workspace's own gitconfig can enable it — does git actually omit the
// leading space and emit a truly empty line, which is what this branch exists
// to count correctly rather than mistaking for the hunk having ended.
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
	// One correctly-sized 5-line hunk is the proof the blank line was counted
	// as context rather than closing the hunk early and reopening a second,
	// spurious one from what remained.
	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 1, OldLines: 5, NewStart: 1, NewLines: 5},
	}, files[0].Hunks)
}
