package branchreview_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The git helpers below are intentionally small duplicates of the ones in the
// worktree package's bench test. They are copied (not shared) so branchreview
// stays an independent package with no test-time dependency on worktree.

func benchInitRepo(
	b *testing.B,
) string {
	b.Helper()
	dir := b.TempDir()
	benchGitRun(b, dir, "init")
	benchGitRun(b, dir, "config", "user.email", "t@t")
	benchGitRun(b, dir, "config", "user.name", "t")
	benchGitRun(b, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

func benchGitRun(
	b *testing.B,
	dir string,
	args ...string,
) string {
	b.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		cmd.Environ(),
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(b, err, string(out))
	return string(out)
}

func benchCommitInWorktree(
	b *testing.B,
	worktree string,
	file string,
	content string,
	msg string,
) {
	b.Helper()
	path := filepath.Join(worktree, file)
	cmd := exec.Command("sh", "-c", "printf '%s' "+shellQuote(content)+" > "+shellQuote(path))
	require.NoError(b, cmd.Run())
	benchGitRun(b, worktree, "add", file)
	benchGitRun(b, worktree, "commit", "-m", msg)
}

func shellQuote(
	s string,
) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'\''`
			continue
		}
		out += string(r)
	}
	return out + "'"
}

func trimNewline(
	s string,
) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
