package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// gitRepo runs git against exactly one repository and cannot reach any other.
//
// That guarantee is the whole point of the type. The throwaway fixture lives
// under CROWBAR_HOME, and on a dev machine CROWBAR_HOME is <workspace>/.crowbar
// — a directory inside a Crowbar checkout, which is itself a linked worktree
// whose .git is a file. Git's repository discovery walks UP, so a seed command
// aimed at a directory that is not, or is no longer, a repository silently
// resolves the enclosing Crowbar source repository and reads — or commits —
// there. Two independent fences make that impossible rather than unlikely:
//
//   - fenceEnv pins git's upward search to the repository root, so a miss
//     fails with "not a git repository" instead of escaping.
//   - verifyToplevel asks git which worktree it actually resolved and refuses
//     anything else, so a run whose ceiling did not apply — a symlinked path,
//     an inherited GIT_DIR — is still caught.
type gitRepo struct {
	root string
}

// gitEnvOverrides are the inherited variables that would let git resolve a
// repository the seed did not choose. GIT_DIR and its relatives bypass
// discovery outright, and an inherited ceiling would win over the appended one
// on platforms whose getenv answers with the first match rather than the last.
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

// commonDir is the repository directory a worktree shares with every other
// worktree of the same repository. Comparing it is what proves that a path the
// daemon handed the seed really belongs to the fixture, and not to whatever
// checkout happens to enclose it.
func (g *gitRepo) commonDir() (string, error) {
	out, err := g.output("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return realPath(strings.TrimSpace(out))
}

// verifyToplevel is the fence that does not depend on git honouring an
// environment variable: it asks git which worktree root it just resolved and
// refuses anything but dir. env is a parameter so the check can be exercised
// with the ceiling deliberately absent.
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

// fenceEnv stops git's repository discovery from climbing above dir.
// GIT_CEILING_DIRECTORIES names directories git refuses to chdir up into, and
// the search starts at the working directory itself, so listing dir's parent
// leaves exactly one candidate: dir.
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

// realPath is how two paths are compared anywhere in this package. macOS routes
// both the temp dir and the home dir through symlinks, so a raw string compare
// between what the caller asked for and what git answered would fail on paths
// that are in fact the same directory.
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
