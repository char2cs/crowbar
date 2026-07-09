package git

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WorkspaceSyncer is the workspace surface the git usecase uses to resolve a
// worktree path and trigger a working-tree resync after a mutation. The git
// package keeps its own narrow copy so it never imports the file or workspace
// usecase; the container passes the concrete workspace usecase, which satisfies it.
type WorkspaceSyncer interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	ResolveConflicts(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
}

// ReadEngine is the read surface the git usecase passes through to.
type ReadEngine interface {
	Status(
		ctx context.Context,
		repoPath string,
	) (gitdomain.GitStatus, error)
	PullIsFastForward(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)
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
	ConflictedFiles(
		ctx context.Context,
		repoPath string,
	) ([]string, error)
	ConflictHunks(
		ctx context.Context,
		repoPath string,
		filePath string,
	) ([]gitdomain.ConflictHunk, error)
}

// Usecase is the lifecycle/read git surface over a workspace's worktree.
// Read methods pass through; mutating methods additionally trigger a working-tree
// resync (00 §5.3).
type Usecase interface {
	Status(
		ctx context.Context,
		wsID string,
	) (gitdomain.GitStatus, error)
	PullIsFastForward(
		ctx context.Context,
		wsID string,
	) (bool, error)
	Diff(
		ctx context.Context,
		wsID string,
		staged bool,
	) ([]gitdomain.FileDiff, error)
	CommitDiff(
		ctx context.Context,
		wsID string,
		sha string,
	) (gitdomain.MultiFileDiff, error)
	Log(
		ctx context.Context,
		wsID string,
		limit int,
		skip int,
	) ([]gitdomain.Commit, error)
	Blame(
		ctx context.Context,
		wsID string,
		filePath string,
	) ([]gitdomain.BlameEntry, error)
	Branches(
		ctx context.Context,
		wsID string,
	) ([]gitdomain.Branch, error)
	Stashes(
		ctx context.Context,
		wsID string,
	) ([]gitdomain.Stash, error)
	ConflictedFiles(
		ctx context.Context,
		wsID string,
	) ([]string, error)
	ConflictHunks(
		ctx context.Context,
		wsID string,
		filePath string,
	) ([]gitdomain.ConflictHunk, error)

	WriteUsecase
}

// WriteUsecase is the mutating git surface; every method resyncs after the op.
type WriteUsecase interface {
	StageFile(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
	StageHunk(
		ctx context.Context,
		wsID string,
		filePath string,
		hunkID string,
		now time.Time,
	) error
	UnstageFile(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
	UnstageHunk(
		ctx context.Context,
		wsID string,
		filePath string,
		hunkID string,
		now time.Time,
	) error
	Discard(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
	Commit(
		ctx context.Context,
		wsID string,
		subject string,
		body string,
		now time.Time,
	) error
	Push(
		ctx context.Context,
		wsID string,
		now time.Time,
	) error
	Fetch(
		ctx context.Context,
		wsID string,
		now time.Time,
	) error
	Pull(
		ctx context.Context,
		wsID string,
		mode string,
		now time.Time,
	) error
	CreateBranch(
		ctx context.Context,
		wsID string,
		name string,
		source string,
		switchTo bool,
		now time.Time,
	) error
	RenameBranch(
		ctx context.Context,
		wsID string,
		oldName string,
		newName string,
		now time.Time,
	) error
	DeleteBranch(
		ctx context.Context,
		wsID string,
		name string,
		now time.Time,
	) error
	SwitchBranch(
		ctx context.Context,
		wsID string,
		name string,
		now time.Time,
	) error
	StashPush(
		ctx context.Context,
		wsID string,
		message string,
		now time.Time,
	) error
	StashApply(
		ctx context.Context,
		wsID string,
		id string,
		now time.Time,
	) error
	StashPop(
		ctx context.Context,
		wsID string,
		id string,
		now time.Time,
	) error
	StashDrop(
		ctx context.Context,
		wsID string,
		id string,
		now time.Time,
	) error
	Reset(
		ctx context.Context,
		wsID string,
		mode string,
		commit string,
		now time.Time,
	) error
	Merge(
		ctx context.Context,
		wsID string,
		branch string,
		now time.Time,
	) error
	Rebase(
		ctx context.Context,
		wsID string,
		onto string,
		now time.Time,
	) error
	ResolveHunk(
		ctx context.Context,
		wsID string,
		filePath string,
		hunkID string,
		resolution gitdomain.ConflictResolution,
		resolvedContent string,
		now time.Time,
	) error
	OperationContinue(
		ctx context.Context,
		wsID string,
		now time.Time,
	) error
	OperationAbort(
		ctx context.Context,
		wsID string,
		now time.Time,
	) error
}

type gitUsecase struct {
	git    OpsEngine
	syncer WorkspaceSyncer
}

// New builds a Usecase from the git engine and the workspace syncer.
func New(
	git OpsEngine,
	syncer WorkspaceSyncer,
) Usecase {
	return &gitUsecase{
		git:    git,
		syncer: syncer,
	}
}

func (u *gitUsecase) repoPath(
	ctx context.Context,
	wsID string,
) (string, error) {
	ws, err := u.syncer.Get(ctx, wsID)
	if err != nil {
		return "", fmt.Errorf("git: load workspace: %w", err)
	}
	return ws.WorktreePath, nil
}

func (u *gitUsecase) writePath(
	ctx context.Context,
	wsID string,
) (string, error) {
	ws, err := u.syncer.Get(ctx, wsID)
	if err != nil {
		return "", fmt.Errorf("git: load workspace: %w", err)
	}
	if ws.Status == domain.WorkspaceStatusLocked {
		return "", fmt.Errorf("git: mutate: workspace locked: %w", apperr.ErrLocked)
	}
	return ws.WorktreePath, nil
}

// Status returns the working-tree status of a workspace.
func (u *gitUsecase) Status(
	ctx context.Context,
	wsID string,
) (gitdomain.GitStatus, error) {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return gitdomain.GitStatus{}, err
	}
	st, err := u.git.Status(ctx, repoPath)
	if err != nil {
		return gitdomain.GitStatus{}, fmt.Errorf("git: status: %w", err)
	}
	return st, nil
}

// PullIsFastForward reports whether a pull on the workspace's branch would fast
// forward — true iff the local branch holds no commit its upstream lacks. It is
// the synchronous safety gate the pull handler consults before accepting the
// pull: a false result means the branch has diverged and the pull is refused
// instead of blindly merging. The check resolves the workspace's write path
// (rejecting a locked workspace, as the pull itself would) and its current
// branch, then defers to the local, no-network engine check.
func (u *gitUsecase) PullIsFastForward(
	ctx context.Context,
	wsID string,
) (bool, error) {
	repoPath, err := u.writePath(ctx, wsID)
	if err != nil {
		return false, err
	}
	st, err := u.git.Status(ctx, repoPath)
	if err != nil {
		return false, fmt.Errorf("git: pull ff-check: status: %w", err)
	}
	ff, err := u.git.PullIsFastForward(ctx, repoPath, st.Branch)
	if err != nil {
		return false, fmt.Errorf("git: pull ff-check: %w", err)
	}
	return ff, nil
}

// Diff returns the working-tree (or staged) diff of a workspace.
func (u *gitUsecase) Diff(
	ctx context.Context,
	wsID string,
	staged bool,
) ([]gitdomain.FileDiff, error) {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return nil, err
	}
	diff, err := u.git.Diff(ctx, repoPath, staged)
	if err != nil {
		return nil, fmt.Errorf("git: diff: %w", err)
	}
	return diff, nil
}

// CommitDiff returns the diff for a single commit. sha is validated as a safe
// operand so a leading-dash / metacharacter value can never be read by `git
// show`/`git diff` as an option such as `--output=<path>` (argument injection →
// arbitrary file write); `git show` cannot hide a revision behind `--`, so the
// validator is the guard here.
func (u *gitUsecase) CommitDiff(
	ctx context.Context,
	wsID string,
	sha string,
) (gitdomain.MultiFileDiff, error) {
	if err := validateOperand("sha", sha); err != nil {
		return gitdomain.MultiFileDiff{}, err
	}
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return gitdomain.MultiFileDiff{}, err
	}
	diff, err := u.git.CommitDiff(ctx, repoPath, sha)
	if err != nil {
		return gitdomain.MultiFileDiff{}, fmt.Errorf("git: commit diff: %w", err)
	}
	return diff, nil
}

// Log returns paginated git log from HEAD.
func (u *gitUsecase) Log(
	ctx context.Context,
	wsID string,
	limit int,
	skip int,
) ([]gitdomain.Commit, error) {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return nil, err
	}
	commits, err := u.git.Log(ctx, repoPath, limit, skip)
	if err != nil {
		return nil, fmt.Errorf("git: log: %w", err)
	}
	return commits, nil
}

// Blame annotates each line of a file with its last-changing commit.
func (u *gitUsecase) Blame(
	ctx context.Context,
	wsID string,
	filePath string,
) ([]gitdomain.BlameEntry, error) {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return nil, err
	}
	entries, err := u.git.Blame(ctx, repoPath, filePath)
	if err != nil {
		return nil, fmt.Errorf("git: blame: %w", err)
	}
	return entries, nil
}

// Branches returns all branches.
func (u *gitUsecase) Branches(
	ctx context.Context,
	wsID string,
) ([]gitdomain.Branch, error) {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return nil, err
	}
	branches, err := u.git.Branches(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("git: branches: %w", err)
	}
	return branches, nil
}

// Stashes returns all stash entries.
func (u *gitUsecase) Stashes(
	ctx context.Context,
	wsID string,
) ([]gitdomain.Stash, error) {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return nil, err
	}
	stashes, err := u.git.Stashes(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("git: stashes: %w", err)
	}
	return stashes, nil
}

// ConflictedFiles returns paths with merge conflicts.
func (u *gitUsecase) ConflictedFiles(
	ctx context.Context,
	wsID string,
) ([]string, error) {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return nil, err
	}
	files, err := u.git.ConflictedFiles(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("git: conflicted files: %w", err)
	}
	return files, nil
}

// ConflictHunks parses the three-way conflict view for a file.
func (u *gitUsecase) ConflictHunks(
	ctx context.Context,
	wsID string,
	filePath string,
) ([]gitdomain.ConflictHunk, error) {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return nil, err
	}
	hunks, err := u.git.ConflictHunks(ctx, repoPath, filePath)
	if err != nil {
		return nil, fmt.Errorf("git: conflict hunks: %w", err)
	}
	return hunks, nil
}
