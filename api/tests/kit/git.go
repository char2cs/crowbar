//go:build integration || manual

package kit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// GitRepo is a real git repository created in a temp directory.
// Use NewGitRepo to create one; use Commit to add commits with specific files.
type GitRepo struct {
	t    *testing.T
	path string
}

// NewGitRepo creates a real git repository in t.TempDir with an initial empty
// commit. The repository is configured with a test author so git operations
// never prompt for user identity.
func NewGitRepo(
	t *testing.T,
) *GitRepo {
	t.Helper()
	dir := t.TempDir()
	r := &GitRepo{t: t, path: dir}
	r.git("init")
	r.git("config", "user.email", "test@test.com")
	r.git("config", "user.name", "Test")
	// Create an initial commit so the repo has a HEAD and a main branch.
	r.git("commit", "--allow-empty", "-m", "initial")
	return r
}

// Path returns the absolute path of the repository on disk.
func (r *GitRepo) Path() string {
	return r.path
}

// Commit writes files to disk, stages all changes, and commits with the given
// message. It returns the full commit SHA.
func (r *GitRepo) Commit(
	message string,
	files map[string]string,
) string {
	r.t.Helper()
	for name, content := range files {
		fullPath := filepath.Join(r.path, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			r.t.Fatalf("GitRepo.Commit: mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			r.t.Fatalf("GitRepo.Commit: write %s: %v", name, err)
		}
	}
	r.git("add", "-A")
	r.git("commit", "-m", message)
	return strings.TrimSpace(r.gitOutput("rev-parse", "HEAD"))
}

func (r *GitRepo) git(
	args ...string,
) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func (r *GitRepo) gitOutput(
	args ...string,
) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.path
	out, err := cmd.Output()
	if err != nil {
		r.t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return fmt.Sprintf("%s", out)
}
