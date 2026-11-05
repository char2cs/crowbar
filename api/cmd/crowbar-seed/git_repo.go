package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// gitRepo runs git against exactly one repository. The fixture lives under a dev
// CROWBAR_HOME inside a Crowbar checkout, so git's upward discovery would otherwise
// reach the Crowbar repo; fenceEnv and verifyToplevel are two independent fences.
type gitRepo struct {
	root string
}

// gitEnvOverrides are inherited variables that would let git resolve a repo the
// seed did not choose: GIT_DIR and relatives bypass discovery, and an inherited
// ceiling can win over the appended one where getenv returns the first match.
var gitEnvOverrides = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_CEILING_DIRECTORIES",
	"GIT_COMMON_DIR",
	"GIT_DIR",
	"GIT_DISCOVERY_ACROSS_FILESYSTEM",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_WORK_TREE",
}

// openRepo fences git to the repository whose worktree root is root, refusing a
// path that resolves anywhere else. Every git command the seed runs goes
// through the value it returns.
func openRepo(
	root string,
) (*gitRepo, error) {
	resolved, err := realPath(root)
	if err != nil {
		return nil, err
	}
	if err := verifyToplevel(resolved, fenceEnv(resolved)); err != nil {
		return nil, err
	}
	return &gitRepo{root: resolved}, nil
}

func (g *gitRepo) run(
	args ...string,
) error {
	_, err := g.output(args...)
	return err
}

func (g *gitRepo) output(
	args ...string,
) (string, error) {
	return runGit(g.root, args...)
}

// commonDir is the repository directory a worktree shares with its siblings;
// comparing it proves a daemon-reported path belongs to the fixture.
func (g *gitRepo) commonDir() (string, error) {
	out, err := g.output("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return realPath(strings.TrimSpace(out))
}

// verifyToplevel asks git which worktree root it resolved and refuses anything
// but dir, a fence that does not rely on git honouring an env var. env is a
// parameter so the check can be exercised with the ceiling absent.
func verifyToplevel(
	dir string,
	env []string,
) error {
	top, err := gitWithEnv(dir, env, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	resolved, err := realPath(strings.TrimSpace(top))
	if err != nil {
		return err
	}
	if resolved != dir {
		return fmt.Errorf("seed: refusing to run git in %s: it resolves to the repository at %s", dir, resolved)
	}
	return nil
}

// fenceEnv stops git's repository discovery from climbing above dir: listing
// dir's parent in GIT_CEILING_DIRECTORIES leaves dir as the only candidate.
func fenceEnv(
	dir string,
) []string {
	inherited := os.Environ()
	env := make([]string, 0, len(inherited)+1)
	for _, entry := range inherited {
		if !isGitOverride(entry) {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
}

func isGitOverride(
	entry string,
) bool {
	name, _, _ := strings.Cut(entry, "=")
	return slices.Contains(gitEnvOverrides, name)
}

func runGit(
	dir string,
	args ...string,
) (string, error) {
	return gitWithEnv(dir, fenceEnv(dir), args...)
}

// gitWithEnv executes git in dir. Stderr is folded into the error because git
// says why it refused there, and a seed that fails silently on "not a git
// repository" is worse than no seed at all.
func gitWithEnv(
	dir string,
	env []string,
	args ...string,
) (string, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // G204: args are seed-authored literals, never user input
	cmd.Dir = dir
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf(
			"seed: git %s in %s: %w: %s",
			strings.Join(args, " "), dir, err, strings.TrimSpace(stderr.String()),
		)
	}
	return string(out), nil
}

// realPath is how this package compares paths: macOS routes the temp and home
// dirs through symlinks, so a raw string compare would miss identical directories.
func realPath(
	path string,
) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("seed: resolve %s: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("seed: resolve %s: %w", path, err)
	}
	return resolved, nil
}
