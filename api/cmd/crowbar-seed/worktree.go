package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const worktreeRegistryDir = "worktrees"

// ensureWorktree opens the workspace worktree at path as a worktree of fixture,
// repairing it first if an earlier run left it dangling, and reports whether it
// had to.
//
// Crowbar provisions that worktree from the fixture repo, which registers it at
// <fixture>/.git/worktrees/<name>. Regenerating the fixture, or moving and
// removing workspaces, leaves the two halves of the registration out of step,
// and every git command in the worktree then dies with
// "fatal: not a git repository: .../.git/worktrees/<name>". The seed owns the
// whole fixture, so it can put the two halves back together instead of asking a
// developer to delete their dev state.
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

// openWorktree opens path only if it is a worktree of fixture. The seed commits
// into whatever this returns, and .crowbar/ sits inside a Crowbar checkout, so
// "some repository" is not good enough — it has to be the fixture's.
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

// repairWorktree rebuilds the worktree at path from the fixture repo.
//
// Neither half of a broken registration heals on its own: `git worktree prune`
// only drops registrations whose directory is gone, `git worktree repair`
// refuses a .git file that references no repository, and `git worktree add`
// rejects an existing path even with --force. So both halves are cleared and
// the worktree is created again.
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

// clearOrphan removes the directory a vanished registration left behind.
// os.RemoveAll on a path the daemon merely reported is only safe once the
// directory has proved it is a worktree of this fixture whose registration is
// genuinely gone; anything else is left untouched and refused by name.
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

// addWorktree checks branch out at path, reusing the branch when the fixture
// still carries it so a repair does not discard the review commit.
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

// orphanedOf reports whether path is a worktree of fixture whose registration
// inside the fixture repo no longer exists.
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

// worktreeGitdir reads the `gitdir:` pointer a linked worktree keeps in place of
// a .git directory, answering "" for anything that is not one.
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
