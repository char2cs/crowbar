package git

import (
	"context"
	"log/slog"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// worktrees is the worktree surface of Engine: adding, listing, moving and removing
// checkouts, and counting what a removal would lose.
type worktrees interface {
	// WorktreeAdd adds a git worktree for branch at path (04 write ops / 07).
	WorktreeAdd(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
	) error

	// WorktreeRemove removes a git worktree. Without force git refuses a dirty worktree, the only safe way
	// to remove one whose work is not Crowbar's to throw away.
	WorktreeRemove(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		force bool,
	) error

	// WorktreeRepair repoints the repo's admin files at a worktree already moved on disk, so git follows
	// a branch rename that relocated the workspace root.
	WorktreeRepair(
		ctx context.Context,
		repoPath string,
		worktreePath string,
	) error

	// WorktreePrune reaps worktree registrations whose on-disk directory is gone
	// (`git worktree prune`). Used by the startup recovery sweep to clean up
	// orphaned worktrees left behind by a crash mid-teardown (H19).
	WorktreePrune(
		ctx context.Context,
		repoPath string,
	) error

	// UncommittedFiles counts the paths git status reports in the worktree at
	// worktreePath — staged, unstaged and untracked, never ignored: exactly what
	// `git worktree remove --force` would throw away.
	UncommittedFiles(
		ctx context.Context,
		worktreePath string,
	) (int, error)

	// UnmergedCommits counts commits from tips that nothing but dropBranch reaches (no other branch,
	// remote branch or tag): what is lost once dropBranch and its worktree go. Missing tips are ignored.
	UnmergedCommits(
		ctx context.Context,
		dir string,
		tips []string,
		dropBranch string,
	) (int, error)

	// WorktreeList lists all git worktrees in a repo (04 / 07).
	WorktreeList(
		ctx context.Context,
		repoPath string,
	) ([]WorktreeEntry, error)

	// DetachWorktree detaches HEAD of the worktree at worktreePath (keeping its
	// files), freeing the branch it had checked out so another worktree can take
	// it (07 — managed-worktree import of the unmanaged main folder's branch).
	DetachWorktree(
		ctx context.Context,
		worktreePath string,
	) error

	// CheckoutBranch switches the worktree at worktreePath onto branch,
	// re-attaching a detached HEAD (rollback / restore the main folder).
	CheckoutBranch(
		ctx context.Context,
		worktreePath string,
		branch string,
	) error

	// WorktreeAddBranch creates a new git worktree at worktreePath on a freshly
	// created branch starting at startPoint, returning the resolved start SHA so
	// callers can record it as forkPointSha (07 §1).
	WorktreeAddBranch(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
		startPoint string,
	) (string, error)

	// WorktreeAddAtRef checks branch out into a fresh worktree with the ref RESET to startRef
	// (`git worktree add -B`), returning the start SHA, so an import holds startRef's commits even
	// when a diverged local branch of that name exists.
	WorktreeAddAtRef(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
		startRef string,
	) (string, error)
}

// WorktreeEntry is a single worktree from `git worktree list`. Prunable is true
// when git reports the worktree's working directory as missing (a dangling
// registration left by a deleted checkout); such entries are not usable.
type WorktreeEntry struct {
	Path     string
	Branch   string
	Head     string
	Prunable bool
}

func (e *engine) WorktreeAdd(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "worktree", "add", worktreePath, branch)
	if rawErr := gitexec.RequireSuccess("worktree add", r); rawErr != nil && isStaleWorktreeConflict(rawErr) {
		// A worktree whose directory was removed out from under git still holds
		// its branch "checked out", which blocks re-adding that branch with
		// "already used by worktree" — so importing that branch fails forever.
		// Prune dead registrations and retry once. `git worktree prune` only
		// removes worktrees whose directory is gone, so a genuine conflict with a
		// live worktree still fails on the retry (correctly).
		e.pruneBeforeRetry(ctx, repoPath)
		r = e.exec(ctx, repoPath, "worktree", "add", worktreePath, branch)
	}
	return classifyGitError("worktree add", r)
}

// isStaleWorktreeConflict reports whether a worktree-add error is the
// "branch already used / checked out by another worktree" failure that pruning
// dead worktree registrations can clear.
func isStaleWorktreeConflict(err error) bool {
	if err == nil {
		return false
	}
	m := err.Error()
	return strings.Contains(m, "already used by worktree") ||
		strings.Contains(m, "is already checked out")
}

func (e *engine) WorktreeRemove(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	force bool,
) error {
	defer e.lockRepo(ctx, repoPath)()
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	r := e.exec(ctx, repoPath, append(args, worktreePath)...)
	return gitexec.RequireSuccess("worktree remove", r)
}

// WorktreeRepair repoints the repo's administrative files at a worktree that
// has already been relocated on disk (`git worktree repair`), fixing both the
// registration's gitdir and the worktree's own .git link.
//
// Branch rename moves a workspace by renaming its whole ROOT directory in one
// os.Rename — because the root holds the git worktree and the agent `chats`
// tree as siblings, and those must travel together or the chat history is
// orphaned at the old branch's path. `git worktree move` cannot express that:
// it relocates only the worktree leaf. So the move is done by the caller and
// git is told to catch up here.
func (e *engine) WorktreeRepair(
	ctx context.Context,
	repoPath string,
	worktreePath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "worktree", "repair", worktreePath)
	return gitexec.RequireSuccess("worktree repair", r)
}

// WorktreePrune runs `git worktree prune`, which removes registrations for
// worktrees whose directory is gone. It only touches dead registrations, so a
// live worktree is never disturbed.
func (e *engine) WorktreePrune(
	ctx context.Context,
	repoPath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.pruneWorktrees(ctx, repoPath)
}

// pruneWorktrees runs `git worktree prune`. The caller holds the repo lock.
func (e *engine) pruneWorktrees(
	ctx context.Context,
	repoPath string,
) error {
	r := e.exec(ctx, repoPath, "worktree", "prune")
	return gitexec.RequireSuccess("worktree prune", r)
}

// pruneBeforeRetry clears dead registrations ahead of a second worktree add. A prune
// that fails is logged, not returned: the add that follows is what decides the outcome.
func (e *engine) pruneBeforeRetry(
	ctx context.Context,
	repoPath string,
) {
	if err := e.pruneWorktrees(ctx, repoPath); err != nil {
		slog.Warn("git: stale worktree registrations not pruned", "repo", repoPath, "err", err)
	}
}

func (e *engine) WorktreeList(
	ctx context.Context,
	repoPath string,
) ([]WorktreeEntry, error) {
	r := e.exec(ctx, repoPath, "worktree", "list", "--porcelain")
	if err := gitexec.RequireSuccess("worktree list", r); err != nil {
		return nil, err
	}
	return parseWorktreeList(r.Stdout), nil
}

// DetachWorktree puts the worktree at worktreePath on a detached HEAD at its
// current commit, freeing whatever branch it had checked out so that branch can
// be added to a NEW worktree. Working-tree files are left untouched (this only
// moves HEAD). Used to free the repo's unmanaged main folder from a branch the
// user wants to open as a real managed worktree.
func (e *engine) DetachWorktree(
	ctx context.Context,
	worktreePath string,
) error {
	defer e.lockRepo(ctx, worktreePath)()
	r := e.exec(ctx, worktreePath, "switch", "--detach")
	return gitexec.RequireSuccess("switch --detach", r)
}

// CheckoutBranch switches the worktree at worktreePath onto branch, re-attaching
// a detached HEAD. Used to roll back a failed detach+add, and to restore the
// main folder once the managed worktree that held its branch is removed. Fails
// if the branch is currently checked out in another worktree.
func (e *engine) CheckoutBranch(
	ctx context.Context,
	worktreePath string,
	branch string,
) error {
	defer e.lockRepo(ctx, worktreePath)()
	r := e.exec(ctx, worktreePath, "switch", branch)
	return classifyGitError("switch", r)
}

func (e *engine) RebaseOnto(ctx context.Context, repoPath, newTip, forkPoint, branch string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.stoppable(ctx, repoPath, "rebase --onto", "rebase", "--onto", newTip, forkPoint, branch)
}

func (e *engine) MergeFFOnly(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "merge", "--ff-only", branch)
	return classifyGitError("merge --ff-only", r)
}

func parseWorktreeList(
	output string,
) []WorktreeEntry {
	var entries []WorktreeEntry
	var current WorktreeEntry
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				entries = append(entries, current)
				current = WorktreeEntry{}
			}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
			continue
		}
		if strings.HasPrefix(line, "HEAD ") {
			current.Head = strings.TrimPrefix(line, "HEAD ")
			continue
		}
		if strings.HasPrefix(line, "branch refs/heads/") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			continue
		}
		if line == "prunable" || strings.HasPrefix(line, "prunable ") {
			current.Prunable = true
		}
	}
	if current.Path != "" {
		entries = append(entries, current)
	}
	return entries
}
