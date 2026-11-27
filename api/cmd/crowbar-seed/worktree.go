package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const worktreeRegistryDir = "worktrees"

func ensureWorktree(
	fixture *gitRepo,
	path string,
	branch string,
) (*gitRepo, bool, error) {
	worktree, err := openWorktree(fixture, path)
	if err == nil {
		return worktree, false, nil
	}
	if err := repairWorktree(fixture, path, branch); err != nil {
		return nil, false, err
	}
	repaired, err := openWorktree(fixture, path)
	return repaired, true, err
}

func openWorktree(
	fixture *gitRepo,
	path string,
) (*gitRepo, error) {
	worktree, err := openRepo(path)
	if err != nil {
		return nil, err
	}
	mine, err := fixture.commonDir()
	if err != nil {
		return nil, err
	}
	theirs, err := worktree.commonDir()
	if err != nil {
		return nil, err
	}
	if mine != theirs {
		return nil, fmt.Errorf(
			"seed: %s is a worktree of %s, not of the fixture at %s",
			path, theirs, fixture.root,
		)
	}
	return worktree, nil
}

func repairWorktree(
	fixture *gitRepo,
	path string,
	branch string,
) error {
	if err := fixture.run("worktree", "prune"); err != nil {
		return err
	}
	if err := clearOrphan(fixture, path); err != nil {
		return err
	}
	return addWorktree(fixture, path, branch)
}

func clearOrphan(
	fixture *gitRepo,
	path string,
) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	}
	orphaned, err := orphanedOf(fixture, path)
	if err != nil {
		return err
	}
	if !orphaned {
		return fmt.Errorf(
			"seed: %s already exists and is not an orphaned worktree of the fixture at %s; remove it by hand",
			path, fixture.root,
		)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("seed: remove the orphaned worktree at %s: %w", path, err)
	}
	return nil
}

func addWorktree(
	fixture *gitRepo,
	path string,
	branch string,
) error {
	if err := fixture.run("show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return fixture.run("worktree", "add", path, branch)
	}
	return fixture.run("worktree", "add", "-b", branch, path, seedBaseBranch)
}

func orphanedOf(
	fixture *gitRepo,
	path string,
) (bool, error) {
	target, err := worktreeGitdir(path)
	if err != nil || target == "" {
		return false, err
	}
	registry := filepath.Dir(target)
	if filepath.Base(registry) != worktreeRegistryDir {
		return false, nil
	}
	owner, ownerErr := realPath(filepath.Dir(registry))
	common, err := fixture.commonDir()
	if err != nil {
		return false, err
	}
	if ownerErr != nil || owner != common {
		return false, nil
	}
	_, err = os.Stat(target)
	return os.IsNotExist(err), nil
}

func worktreeGitdir(
	path string,
) (string, error) {
	marker := filepath.Join(path, ".git")
	info, err := os.Lstat(marker)
	if err != nil || !info.Mode().IsRegular() {
		return "", nil
	}
	body, err := os.ReadFile(marker) //nolint:gosec // G304: the path is the seed's own fixture worktree
	if err != nil {
		return "", fmt.Errorf("seed: read %s: %w", marker, err)
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(body)), "gitdir:")
	if !ok {
		return "", nil
	}
	return filepath.Clean(strings.TrimSpace(target)), nil
}
