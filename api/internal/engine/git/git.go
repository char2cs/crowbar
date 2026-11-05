// Package git is the git execution engine (04). It shells out to the system
// git binary and is consumed by app/usecases. Stateless per call.
package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Engine is the full git operation surface.
type Engine interface {
	worktrees
	reviewReads

	// Status computes the current GitStatus for a workspace.
	Status(
		ctx context.Context,
		repoPath string,
	) (gitdomain.GitStatus, error)

	// Diff returns the working-tree diff for a workspace.
	// If staged is true, returns the staged diff (--cached).
	Diff(
		ctx context.Context,
		repoPath string,
		staged bool,
	) ([]gitdomain.FileDiff, error)

	// CommitDiff returns the diff for a single commit.
	CommitDiff(
		ctx context.Context,
		repoPath string,
		sha string,
	) (gitdomain.MultiFileDiff, error)

	// Log returns paginated git log from HEAD.
	Log(
		ctx context.Context,
		repoPath string,
		limit int,
		skip int,
	) ([]gitdomain.Commit, error)

	// Blame annotates each line of filePath with its last-changing commit.
	Blame(
		ctx context.Context,
		repoPath string,
		filePath string,
	) ([]gitdomain.BlameEntry, error)

	// Branches returns all branches (local + remote).
	Branches(
		ctx context.Context,
		repoPath string,
	) ([]gitdomain.Branch, error)

	// Stashes returns all stash entries.
	Stashes(
		ctx context.Context,
		repoPath string,
	) ([]gitdomain.Stash, error)

	// StageFile stages a file.
	StageFile(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error

	// StageHunk stages a single hunk identified by hunkID.
	StageHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
	) error

	// UnstageFile unstages a file.
	UnstageFile(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error

	// UnstageHunk unstages a single hunk identified by hunkID.
	UnstageHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
	) error

	// Discard discards changes to a file. Tracked-modified files use
	// git restore; untracked files are removed.
	Discard(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error

	// Commit creates a commit with subject and optional body.
	Commit(
		ctx context.Context,
		repoPath string,
		subject string,
		body string,
	) error

	// Push pushes the current branch.
	Push(
		ctx context.Context,
		repoPath string,
	) error

	// Fetch fetches from the remote.
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

	// Pull runs git pull. mode is "merge" or "rebase".
	Pull(
		ctx context.Context,
		repoPath string,
		mode string,
	) error

	// CreateBranch creates a branch. If switchTo is true, also checks it out.
	// source is optional (empty = HEAD).
	CreateBranch(
		ctx context.Context,
		repoPath string,
		name string,
		source string,
		switchTo bool,
	) error

	// RenameBranch renames a branch.
	RenameBranch(
		ctx context.Context,
		repoPath string,
		oldName string,
		newName string,
	) error

	// DeleteBranch deletes a branch with -d (safe).
	DeleteBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error

	// ForceDeleteBranch deletes a branch with -D (workspace teardown).
	ForceDeleteBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error

	// SwitchBranch checks out an existing branch.
	SwitchBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error

	// StashPush stashes the current changes. message is optional.
	StashPush(
		ctx context.Context,
		repoPath string,
		message string,
	) error

	// StashApply applies a stash without removing it.
	StashApply(
		ctx context.Context,
		repoPath string,
		id string,
	) error

	// StashPop applies and removes a stash.
	StashPop(
		ctx context.Context,
		repoPath string,
		id string,
	) error

	// StashDrop removes a stash without applying.
	StashDrop(
		ctx context.Context,
		repoPath string,
		id string,
	) error

	// Reset runs git reset with the given mode (soft|mixed|hard).
	Reset(
		ctx context.Context,
		repoPath string,
		mode string,
		commit string,
	) error

	// Merge merges branch into the current branch; MergeCommit never
	// fast-forwards, so the current branch always gains a merge commit.
	Merge(ctx context.Context, repoPath, branch string) error
	MergeCommit(ctx context.Context, repoPath, branch string) error

	// Rebase rebases the current branch onto onto.
	Rebase(ctx context.Context, repoPath, onto string) error

	// ConflictedFiles returns paths with merge conflicts.
	ConflictedFiles(
		ctx context.Context,
		repoPath string,
	) ([]string, error)

	// ConflictHunks parses three-way conflict view for a file.
	ConflictHunks(
		ctx context.Context,
		repoPath string,
		filePath string,
	) ([]gitdomain.ConflictHunk, error)

	// ResolveHunk resolves a single conflict hunk.
	ResolveHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
		resolution gitdomain.ConflictResolution,
		resolvedContent string,
	) error

	// OperationContinue finalizes an in-progress merge/rebase/pull.
	OperationContinue(
		ctx context.Context,
		repoPath string,
	) error

	// OperationAbort aborts an in-progress merge/rebase/pull.
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

	// RevParse resolves rev to a full commit SHA.
	RevParse(
		ctx context.Context,
		repoPath string,
		rev string,
	) (string, error)

	// MergeSquash runs `git merge --squash branch` then `git commit -m subject`
	// inside the repo lock. Returns ErrConflict on conflicts.
	MergeSquash(
		ctx context.Context,
		repoPath string,
		branch string,
		subject string,
	) error

	// RebaseThenFFMerge replays childWorktree onto parentBranch then fast-forwards
	// parentWorktree to childBranch as one locked unit, so the parent cannot advance
	// between the two steps and break the --ff-only.
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
