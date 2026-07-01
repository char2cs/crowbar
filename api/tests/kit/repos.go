//go:build integration

package kit

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// GitEnv is the minimal git identity environment to avoid identity prompts.
var GitEnv = []string{
	"GIT_AUTHOR_NAME=test",
	"GIT_AUTHOR_EMAIL=test@test.com",
	"GIT_COMMITTER_NAME=test",
	"GIT_COMMITTER_EMAIL=test@test.com",
}

// InitRepo creates a new git repo in a temp dir with one empty initial commit.
func InitRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	GitRun(
		t,
		dir,
		"init",
		"-b",
		"main",
	)
	GitRun(
		t,
		dir,
		"config",
		"user.email",
		"test@test.com",
	)
	GitRun(
		t,
		dir,
		"config",
		"user.name",
		"test",
	)
	GitRun(
		t,
		dir,
		"commit",
		"--allow-empty",
		"-m",
		"init",
	)
	return dir
}

// InitRepoWithFile creates a repo with a committed file at filename.
func InitRepoWithFile(
	t *testing.T,
	filename string,
	content string,
) string {
	t.Helper()
	dir := InitRepo(t)
	CommitFile(
		t,
		dir,
		filename,
		content,
		"add "+filename,
	)
	return dir
}

// CommitFile writes a file and commits it in the given repo.
func CommitFile(
	t *testing.T,
	repoPath string,
	filename string,
	content string,
	msg string,
) {
	t.Helper()
	WriteRepoFile(
		t,
		repoPath,
		filename,
		content,
	)
	GitRun(
		t,
		repoPath,
		"add",
		filename,
	)
	GitRun(
		t,
		repoPath,
		"commit",
		"-m",
		msg,
	)
}

// WriteRepoFile writes content to a path relative to repoPath.
func WriteRepoFile(
	t *testing.T,
	repoPath string,
	filename string,
	content string,
) {
	t.Helper()
	path := filepath.Join(
		repoPath,
		filename,
	)
	require.NoError(
		t,
		os.MkdirAll(
			filepath.Dir(path),
			0o755,
		),
	)
	require.NoError(
		t,
		os.WriteFile(
			path,
			[]byte(content),
			0o644,
		),
	)
}

// BranchName returns the current branch name of the given repo.
func BranchName(
	t *testing.T,
	repoPath string,
) string {
	t.Helper()
	out := GitRun(
		t,
		repoPath,
		"rev-parse",
		"--abbrev-ref",
		"HEAD",
	)
	return TrimNewline(out)
}

// RevParse returns the commit SHA for the given git ref.
func RevParse(
	t *testing.T,
	repoPath string,
	rev string,
) string {
	t.Helper()
	out := GitRun(
		t,
		repoPath,
		"rev-parse",
		rev,
	)
	return TrimNewline(out)
}

// GitRun runs a git command (git <args...>) in dir and returns combined stdout+stderr.
// Fails the test if the command exits non-zero.
func GitRun(
	t testing.TB,
	dir string,
	args ...string,
) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		GitEnv...,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(
		t,
		err,
		"git %v: %s",
		args,
		string(out),
	)
	return string(out)
}

// TrimNewline removes trailing CR/LF characters from s.
func TrimNewline(
	s string,
) string {
	return strings.TrimRight(
		s,
		"\r\n",
	)
}

// InitRepoWithRemoteBranch creates a working repo (default branch main, one
// commit) wired to a fresh bare origin remote, and pushes an extra branch to the
// remote that does NOT exist as a local head. It returns the working repo path
// and the name of the remote-only branch. This is the fixture for the §3
// remote-branch-exists decision: CreateChild on the remote-only branch must
// fetch + checkout it; CreateChild on any absent branch must fork from parent.
func InitRepoWithRemoteBranch(
	t *testing.T,
	remoteBranch string,
) (string, string) {
	t.Helper()
	bare := t.TempDir()
	GitRun(t, bare, "init", "--bare", "-b", "main")

	repo := InitRepoWithFile(t, "base.txt", "base\n")
	GitRun(t, repo, "remote", "add", "origin", bare)
	GitRun(t, repo, "push", "origin", "main")

	// Create the extra branch, push it, then delete the local head so it only
	// exists on the remote (ls-remote --heads origin sees it; local does not).
	GitRun(t, repo, "checkout", "-b", remoteBranch)
	CommitFile(t, repo, "remote-only.txt", "remote-only\n", "remote-only commit")
	GitRun(t, repo, "push", "origin", remoteBranch)
	GitRun(t, repo, "checkout", "main")
	GitRun(t, repo, "branch", "-D", remoteBranch)
	return repo, remoteBranch
}

// BranchExists reports whether branch exists in repoPath.
// Fails the test immediately on unexpected exec errors.
func BranchExists(
	t testing.TB,
	repoPath string,
	branch string,
) bool {
	t.Helper()
	cmd := exec.Command(
		"git",
		"show-ref",
		"--verify",
		"--quiet",
		"refs/heads/"+branch,
	)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), GitEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(
			err,
			&exitErr,
		) {
			return false
		}
		require.NoError(
			t,
			err,
			"git show-ref: %s",
			string(out),
		)
	}
	return true
}

// DirExists reports whether a directory exists at path.
// Fails the test immediately on unexpected stat errors.
func DirExists(
	t testing.TB,
	path string,
) bool {
	t.Helper()
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	require.NoError(
		t,
		err,
	)
	return info.IsDir()
}

// FileExists reports whether a regular file exists at path.
// Fails the test immediately on unexpected stat errors.
func FileExists(
	t testing.TB,
	path string,
) bool {
	t.Helper()
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	require.NoError(
		t,
		err,
	)
	return info.Mode().IsRegular()
}
