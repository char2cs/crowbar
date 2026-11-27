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

	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
)

var GitEnv = []string{
	"GIT_AUTHOR_NAME=test",
	"GIT_AUTHOR_EMAIL=test@test.com",
	"GIT_COMMITTER_NAME=test",
	"GIT_COMMITTER_EMAIL=test@test.com",
}

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

func TrimNewline(
	s string,
) string {
	return strings.TrimRight(
		s,
		"\r\n",
	)
}

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

	GitRun(t, repo, "checkout", "-b", remoteBranch)
	CommitFile(t, repo, "remote-only.txt", "remote-only\n", "remote-only commit")
	GitRun(t, repo, "push", "origin", remoteBranch)
	GitRun(t, repo, "checkout", "main")
	GitRun(t, repo, "branch", "-D", remoteBranch)
	return repo, remoteBranch
}

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

func RepoImport(projectID, repoPath string) project.RepoImport {
	return project.RepoImport{ProjectID: projectID, Path: repoPath}
}

func RequireBranches(tb testing.TB) project.RepoProgress {
	tb.Helper()
	return requireBranches{tb}
}

type requireBranches struct{ t testing.TB }

func (r requireBranches) Workspace(id, branch string, err error) {
	if err != nil {
		r.t.Errorf("import: protected branch %s (workspace %s): %v", branch, id, err)
	}
}
