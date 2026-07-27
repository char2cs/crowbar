// Package git is the git execution engine (04). It shells out to the system
// git binary and is consumed by app/usecases. Stateless per call.
package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Engine is the full git operation surface (04 §2).
type Engine interface {
	// Status computes the current GitStatus for a workspace (04 §3).
	Status(
		ctx context.Context,
		repoPath string,
	) (gitdomain.GitStatus, error)

	// Diff returns the working-tree diff for a workspace (04 §3).
	// If staged is true, returns the staged diff (--cached).
	Diff(
		ctx context.Context,
		repoPath string,
		staged bool,
	) ([]gitdomain.FileDiff, error)

	// CommitDiff returns the diff for a single commit (04 §3).
	CommitDiff(
		ctx context.Context,
		repoPath string,
		sha string,
	) (gitdomain.MultiFileDiff, error)

	// Log returns paginated git log from HEAD (04 §3).
	Log(
		ctx context.Context,
		repoPath string,
		limit int,
		skip int,
	) ([]gitdomain.Commit, error)

	// Blame annotates each line of filePath with its last-changing commit (04 §3).
	Blame(
		ctx context.Context,
		repoPath string,
		filePath string,
	) ([]gitdomain.BlameEntry, error)

	// Branches returns all branches (local + remote) (04 §3).
	Branches(
		ctx context.Context,
		repoPath string,
	) ([]gitdomain.Branch, error)

	// Stashes returns all stash entries (04 §3).
	Stashes(
		ctx context.Context,
		repoPath string,
	) ([]gitdomain.Stash, error)

	// StageFile stages a file (04 §5).
	StageFile(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error

	// StageHunk stages a single hunk identified by hunkID (04 §4, §5).
	StageHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
	) error

	// UnstageFile unstages a file (04 §5).
	UnstageFile(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error

	// UnstageHunk unstages a single hunk identified by hunkID (04 §4, §5).
	UnstageHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
	) error

	// Discard discards changes to a file. Tracked-modified files use
	// git restore; untracked files are removed (04 §5).
	Discard(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error

	// Commit creates a commit with subject and optional body (04 §5).
	Commit(
		ctx context.Context,
		repoPath string,
		subject string,
		body string,
	) error

	// Push pushes the current branch (04 §5).
	Push(
		ctx context.Context,
		repoPath string,
	) error

	// Fetch fetches from the remote (04 §5).
	Fetch(
		ctx context.Context,
		repoPath string,
	) error

	// FetchRef fetches a single branch from the `origin` remote via
	// `git fetch origin <branch>`, making `origin/<branch>` available locally.
	// Used by the worktree usecase before checking out a branch that exists on
	// the remote but not yet locally (07 / Spec §3).
	FetchRef(
		ctx context.Context,
		repoPath string,
		branch string,
	) error

	// FastForwardBranch fetches origin/<branch> and updates the local branch ref
	// to match via `git fetch origin <branch>:<branch>`. Unlike FetchRef, which
	// only updates the remote-tracking ref, this ensures the local branch is at
	// the same commit as origin before being checked out into a worktree.
	FastForwardBranch(
		ctx context.Context,
		repoPath string,
		branch string,
	) error

	// Pull runs git pull. mode is "merge" or "rebase" (04 §5).
	Pull(
		ctx context.Context,
		repoPath string,
		mode string,
	) error

	// CreateBranch creates a branch. If switchTo is true, also checks it out.
	// source is optional (empty = HEAD) (04 §5).
	CreateBranch(
		ctx context.Context,
		repoPath string,
		name string,
		source string,
		switchTo bool,
	) error

	// RenameBranch renames a branch (04 §5).
	RenameBranch(
		ctx context.Context,
		repoPath string,
		oldName string,
		newName string,
	) error

	// DeleteBranch deletes a branch with -d (safe) (04 §5).
	DeleteBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error

	// ForceDeleteBranch deletes a branch with -D (workspace teardown) (04 §5).
	ForceDeleteBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error

	// SwitchBranch checks out an existing branch (04 §5).
	SwitchBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error

	// StashPush stashes the current changes. message is optional (04 §5).
	StashPush(
		ctx context.Context,
		repoPath string,
		message string,
	) error

	// StashApply applies a stash without removing it (04 §5).
	StashApply(
		ctx context.Context,
		repoPath string,
		id string,
	) error

	// StashPop applies and removes a stash (04 §5).
	StashPop(
		ctx context.Context,
		repoPath string,
		id string,
	) error

	// StashDrop removes a stash without applying (04 §5).
	StashDrop(
		ctx context.Context,
		repoPath string,
		id string,
	) error

	// Reset runs git reset with the given mode (soft|mixed|hard) (04 §5).
	Reset(
		ctx context.Context,
		repoPath string,
		mode string,
		commit string,
	) error

	// Merge merges branch into the current branch (04 §5).
	Merge(
		ctx context.Context,
		repoPath string,
		branch string,
	) error

	// Rebase rebases the current branch onto onto (04 §5).
	Rebase(
		ctx context.Context,
		repoPath string,
		onto string,
	) error

	// ConflictedFiles returns paths with merge conflicts (04 §6).
	ConflictedFiles(
		ctx context.Context,
		repoPath string,
	) ([]string, error)

	// ConflictHunks parses three-way conflict view for a file (04 §6).
	ConflictHunks(
		ctx context.Context,
		repoPath string,
		filePath string,
	) ([]gitdomain.ConflictHunk, error)

	// ResolveHunk resolves a single conflict hunk (04 §6).
	ResolveHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
		resolution gitdomain.ConflictResolution,
		resolvedContent string,
	) error

	// OperationContinue finalizes an in-progress merge/rebase/pull (04 §6.1).
	OperationContinue(
		ctx context.Context,
		repoPath string,
	) error

	// OperationAbort aborts an in-progress merge/rebase/pull (04 §6.1).
	OperationAbort(
		ctx context.Context,
		repoPath string,
	) error

	// OperationInProgress reports the in-progress git operation at repoPath, if
	// any: "rebase", "merge", "squash", or "" when the worktree is at rest. It
	// reads the .git marker files (MERGE_HEAD, rebase-merge/, AUTO_MERGE, …) and
	// never mutates. The recovery sweep uses it to tell a workspace that is mid
	// conflict-resolution (markers resolved + staged but the rebase/merge not yet
	// continued) apart from a truly clean tree, so it doesn't prematurely clear a
	// real pr-conflicts state.
	OperationInProgress(
		ctx context.Context,
		repoPath string,
	) (string, error)

	// WorktreeAdd adds a git worktree for branch at path (04 write ops / 07).
	WorktreeAdd(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
	) error

	// WorktreeRemove removes a git worktree (--force) (04 / 07).
	WorktreeRemove(
		ctx context.Context,
		repoPath string,
		worktreePath string,
	) error

	// WorktreeRepair repoints the repo's admin files at a worktree that has
	// already been relocated on disk (`git worktree repair`). Branch rename moves
	// a workspace by renaming its whole root — worktree and agent chats together
	// — then calls this so git follows.
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

	// RebaseOnto runs `git rebase --onto newTip forkPoint branch` (04 / 07).
	RebaseOnto(
		ctx context.Context,
		repoPath string,
		newTip string,
		forkPoint string,
		branch string,
	) error

	// MergeFFOnly runs `git merge --ff-only branch` (04 / 07).
	MergeFFOnly(
		ctx context.Context,
		repoPath string,
		branch string,
	) error

	// WorkingTreeSummary returns +N/-N diff stats plus hasConflicts/hasCommits
	// used by SyncWorkingTreeState (00 §5.3). base is EITHER a base branch name
	// (e.g. "develop") or a fork-point SHA: the summary diffs against the
	// merge-base of base's freshest tip and HEAD, so it counts only the branch's
	// own changes and stays correct as the branch is rebased onto newer base
	// commits — never a frozen fork point that would also count the base branch's
	// advancement.
	WorkingTreeSummary(
		ctx context.Context,
		repoPath string,
		base string,
	) (added, deleted int, hasConflicts, hasCommits bool, err error)

	// MergeBase returns the best common ancestor of commits a and b, used to seed
	// the forkPointSha of adopted worktrees on project import (00 §5.7).
	MergeBase(
		ctx context.Context,
		repoPath string,
		a string,
		b string,
	) (string, error)

	// WouldMergeConflict reports whether a three-way merge of theirs into ours
	// would conflict, computed non-destructively (git merge-tree --write-tree).
	WouldMergeConflict(
		ctx context.Context,
		repoPath string,
		ours string,
		theirs string,
	) (bool, error)

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

	// RevParse resolves rev to a full commit SHA (07 §1 / §3.1).
	RevParse(
		ctx context.Context,
		repoPath string,
		rev string,
	) (string, error)

	// MergeSquash runs `git merge --squash branch` then `git commit -m subject`
	// inside the repo lock (07 §3.1). Returns ErrConflict on conflicts.
	MergeSquash(
		ctx context.Context,
		repoPath string,
		branch string,
		subject string,
	) error

	// RebaseThenFFMerge replays childWorktree onto parentBranch then fast-forwards
	// parentWorktree to childBranch as one locked unit, so the parent cannot advance
	// between the two steps and break the --ff-only (07 §3.1).
	RebaseThenFFMerge(
		ctx context.Context,
		childWorktree string,
		parentBranch string,
		parentWorktree string,
		childBranch string,
	) error

	// RemoteBranchExists reports whether branch exists on the `origin` remote
	// via `git ls-remote --heads origin <branch>` (non-empty output ⇒ true).
	// Used by the worktree usecase to decide checkout-existing vs create-local
	// before adding a worktree (07 / Spec §3).
	RemoteBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)

	// RemoteTrackingBranchExists reports whether the LOCAL remote-tracking ref
	// `refs/remotes/origin/<branch>` is present (`git show-ref --verify`). This is
	// the same universe `git branch -r` reads, so — unlike the live, failure-prone
	// RemoteBranchExists — it agrees with the branch list the import UI shows. The
	// worktree usecase consults it first so a live-query hiccup can never downgrade
	// an existing remote branch into a fresh fork off the default branch.
	RemoteTrackingBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)

	// SetUpstream links a local branch back to its origin counterpart by setting
	// branch.<branch>.remote/merge (`git branch --set-upstream-to=origin/<branch>
	// <branch>`), so the branch is recognised as origin's <branch>. The imported-
	// branch checkout path uses it to make an existing remote branch a proper
	// review target on BOTH the origin-reachable and offline-fallback paths: the
	// reachable path creates the local branch via `git fetch origin <b>:<b>`, which
	// leaves NO upstream, so `git worktree add <path> <b>` checks out that branch
	// untracked — while the fallback path DWIMs tracking from origin/<b>. It
	// operates on the branch by NAME and touches only the shared repo config, so it
	// works whether or not <branch> is repoPath's currently checked-out branch
	// (the caller runs it after checking the branch out into a separate worktree).
	// origin/<branch> must already exist locally as a remote-tracking ref.
	SetUpstream(
		ctx context.Context,
		repoPath string,
		branch string,
	) error

	// RangeDiff returns the three-dot diff between base and branch (09 §2).
	// Uses `git diff -M <base>...<branch>` to show commits reachable from
	// branch but not from base. Commit metadata fields are always zero-value.
	RangeDiff(
		ctx context.Context,
		repoPath string,
		base string,
		branch string,
	) (gitdomain.MultiFileDiff, error)

	// DiffAgainstRef returns the working-tree-inclusive diff against ref:
	// committed changes since ref plus uncommitted tracked modifications
	// (`git diff -M <ref> --`). Used for the blended branch-review diff.
	DiffAgainstRef(
		ctx context.Context,
		repoPath string,
		ref string,
	) (gitdomain.MultiFileDiff, error)

	// ReviewFiles returns the files-only summary of DiffAgainstRef's file set —
	// per-file status + line counts, no hunk content (`git diff --name-status`
	// + `--numstat`). Backs the sidebar's full changed-files list without ever
	// fetching line-level diff content (Task 27).
	//
	// dirty is the set of paths the caller's git status reports as differing
	// from HEAD; passing it keeps the counts off the O(diff size) path. nil
	// means unknown and recomputes every count.
	ReviewFiles(
		ctx context.Context,
		repoPath string,
		ref string,
		dirty []string,
	) ([]gitdomain.ReviewFileSummary, error)
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
