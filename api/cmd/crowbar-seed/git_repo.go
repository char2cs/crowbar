package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type gitRepo struct {
	root string
}

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

func (g *gitRepo) commonDir() (string, error) {
	out, err := g.output("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return realPath(strings.TrimSpace(out))
}

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
