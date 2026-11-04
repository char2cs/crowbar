// Package git is the git execution engine (04). It shells out to the system
// git binary and is consumed by app/usecases. Stateless per call.
package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Engine is the full git operation surface (04 §2).
type Engine interface {
	worktrees
	reviewReads

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

	// FetchRef fetches one branch from origin, making origin/<branch> available locally before
	// checking out a branch that exists only on the remote.
	FetchRef(
		ctx context.Context,
		repoPath string,
		branch string,
	) error

	// FetchPrune refreshes refs/remotes/origin/* and drops gone ones, so a remote-branch listing reflects
	// the remote now; it moves no local branch ref.
	FetchPrune(
		ctx context.Context,
		repoPath string,
	) error

	// FastForwardBranch updates the local branch ref to origin/<branch> (`git fetch origin b:b`), unlike
	// FetchRef, so the branch is at origin's commit before it is checked out.
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

	// Merge merges branch into the current branch (04 §5); MergeCommit never
	// fast-forwards, so the current branch always gains a merge commit.
	Merge(ctx context.Context, repoPath, branch string) error
	MergeCommit(ctx context.Context, repoPath, branch string) error

	// Rebase rebases the current branch onto onto (04 §5).
	Rebase(ctx context.Context, repoPath, onto string) error

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

	// OperationInProgress reports "rebase", "merge", "squash" or "" from the .git marker files, without
	// mutating, so recovery does not clear a pr-conflicts state that is mid-resolution.
	OperationInProgress(
		ctx context.Context,
		repoPath string,
	) (string, error)

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

	// WorkingTreeSummary returns +N/-N stats and conflict/commit flags against the merge-base of base's
	// freshest tip and HEAD (base is a branch or fork-point SHA), so it counts only the branch's own
	// changes and stays right across rebases.
	WorkingTreeSummary(
		ctx context.Context,
		repoPath string,
		base string,
	) (added, deleted int, hasConflicts, hasCommits bool, err error)

	// ResolveDiffBase resolves base to the commit WorkingTreeSummary diffs against; it changes only when
	// refs move, so callers may reuse it across working-tree changes.
	ResolveDiffBase(ctx context.Context, repoPath, base string) gitdomain.DiffBase

	// DiffStat returns the +N/-N of the working tree against commit, as resolved
	// by ResolveDiffBase. An empty commit yields 0, 0.
	DiffStat(
		ctx context.Context,
		repoPath string,
		commit string,
	) (added, deleted int, err error)

	// MergeBase returns the best common ancestor of commits a and b.
	MergeBase(ctx context.Context, repoPath, a, b string) (string, error)

	// WouldMergeConflicts reports per pair whether merging Theirs into Ours would conflict, without
	// touching the tree (merge-tree --write-tree); verdicts are cached by commit.
	WouldMergeConflicts(
		ctx context.Context,
		repoPath string,
		pairs []MergePair,
	) []MergeVerdict

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

	// RemoteBranchExists reports whether branch exists on origin (`git ls-remote --heads`), deciding
	// checkout-existing vs create-local before adding a worktree.
	RemoteBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)

	// RemoteTrackingBranchExists reports whether refs/remotes/origin/<branch> is
	// on disk: unlike the live RemoteBranchExists it agrees with `git branch -r`.
	RemoteTrackingBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)

	// LocalBranchExists reports whether refs/heads/<branch> is present.
	LocalBranchExists(ctx context.Context, repoPath, branch string) (bool, error)

	// SetUpstream sets branch.<branch>.remote/merge to origin/<branch>, which must exist locally. An
	// imported branch created by `git fetch origin b:b` has no upstream, so without this it is checked
	// out untracked and is not a proper review target. It touches only repo config, by branch name.
	SetUpstream(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
}
