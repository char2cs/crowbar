package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type Engine interface {
	worktrees
	reviewReads

	Status(
		ctx context.Context,
		repoPath string,
	) (gitdomain.GitStatus, error)

	Diff(
		ctx context.Context,
		repoPath string,
		staged bool,
	) ([]gitdomain.FileDiff, error)

	CommitDiff(
		ctx context.Context,
		repoPath string,
		sha string,
	) (gitdomain.MultiFileDiff, error)

	Log(
		ctx context.Context,
		repoPath string,
		limit int,
		skip int,
	) ([]gitdomain.Commit, error)

	Blame(
		ctx context.Context,
		repoPath string,
		filePath string,
	) ([]gitdomain.BlameEntry, error)

	Branches(
		ctx context.Context,
		repoPath string,
	) ([]gitdomain.Branch, error)

	Stashes(
		ctx context.Context,
		repoPath string,
	) ([]gitdomain.Stash, error)

	StageFile(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error

	StageHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
	) error

	UnstageFile(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error

	UnstageHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
	) error

	Discard(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error

	Commit(
		ctx context.Context,
		repoPath string,
		subject string,
		body string,
	) error

	Push(
		ctx context.Context,
		repoPath string,
	) error

	Fetch(
		ctx context.Context,
		repoPath string,
	) error

	FetchRef(
		ctx context.Context,
		repoPath string,
		branch string,
	) error

	FetchPrune(
		ctx context.Context,
		repoPath string,
	) error

	FastForwardBranch(
		ctx context.Context,
		repoPath string,
		branch string,
	) error

	Pull(
		ctx context.Context,
		repoPath string,
		mode string,
	) error

	CreateBranch(
		ctx context.Context,
		repoPath string,
		name string,
		source string,
		switchTo bool,
	) error

	RenameBranch(
		ctx context.Context,
		repoPath string,
		oldName string,
		newName string,
	) error

	DeleteBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error

	ForceDeleteBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error

	SwitchBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error

	StashPush(
		ctx context.Context,
		repoPath string,
		message string,
	) error

	StashApply(
		ctx context.Context,
		repoPath string,
		id string,
	) error

	StashPop(
		ctx context.Context,
		repoPath string,
		id string,
	) error

	StashDrop(
		ctx context.Context,
		repoPath string,
		id string,
	) error

	Reset(
		ctx context.Context,
		repoPath string,
		mode string,
		commit string,
	) error

	Merge(ctx context.Context, repoPath, branch string) error
	MergeCommit(ctx context.Context, repoPath, branch string) error

	Rebase(ctx context.Context, repoPath, onto string) error

	ConflictedFiles(
		ctx context.Context,
		repoPath string,
	) ([]string, error)

	ConflictHunks(
		ctx context.Context,
		repoPath string,
		filePath string,
	) ([]gitdomain.ConflictHunk, error)

	ResolveHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
		resolution gitdomain.ConflictResolution,
		resolvedContent string,
	) error

	OperationContinue(
		ctx context.Context,
		repoPath string,
	) error

	OperationAbort(
		ctx context.Context,
		repoPath string,
	) error

	OperationInProgress(
		ctx context.Context,
		repoPath string,
	) (string, error)

	RebaseOnto(
		ctx context.Context,
		repoPath string,
		newTip string,
		forkPoint string,
		branch string,
	) error

	MergeFFOnly(
		ctx context.Context,
		repoPath string,
		branch string,
	) error

	WorkingTreeSummary(
		ctx context.Context,
		repoPath string,
		base string,
	) (added, deleted int, hasConflicts, hasCommits bool, err error)

	ResolveDiffBase(ctx context.Context, repoPath, base string) gitdomain.DiffBase

	DiffStat(
		ctx context.Context,
		repoPath string,
		commit string,
	) (added, deleted int, err error)

	MergeBase(ctx context.Context, repoPath, a, b string) (string, error)

	WouldMergeConflicts(
		ctx context.Context,
		repoPath string,
		pairs []MergePair,
	) []MergeVerdict

	RevParse(
		ctx context.Context,
		repoPath string,
		rev string,
	) (string, error)

	MergeSquash(
		ctx context.Context,
		repoPath string,
		branch string,
		subject string,
	) error

	RebaseThenFFMerge(
		ctx context.Context,
		childWorktree string,
		parentBranch string,
		parentWorktree string,
		childBranch string,
	) error

	RemoteBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)

	RemoteTrackingBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)

	LocalBranchExists(ctx context.Context, repoPath, branch string) (bool, error)

	CheckBranchName(ctx context.Context, repoPath, name string) error

	SetUpstream(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
}
