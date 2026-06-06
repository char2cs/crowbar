package mocks

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// GitOpsEngine is a fake of the full git operation surface used by the git
// usecase. Each method delegates to its Fn field; AnyWriteOK installs no-op
// success closures for every mutating method.
type GitOpsEngine struct {
	StatusFn          func(ctx context.Context, repoPath string) (gitdomain.GitStatus, error)
	DiffFn            func(ctx context.Context, repoPath string, staged bool) ([]gitdomain.FileDiff, error)
	CommitDiffFn      func(ctx context.Context, repoPath, sha string) (gitdomain.MultiFileDiff, error)
	LogFn             func(ctx context.Context, repoPath string, limit, skip int) ([]gitdomain.Commit, error)
	BlameFn           func(ctx context.Context, repoPath, filePath string) ([]gitdomain.BlameEntry, error)
	BranchesFn        func(ctx context.Context, repoPath string) ([]gitdomain.Branch, error)
	StashesFn         func(ctx context.Context, repoPath string) ([]gitdomain.Stash, error)
	ConflictedFilesFn func(ctx context.Context, repoPath string) ([]string, error)
	ConflictHunksFn   func(ctx context.Context, repoPath, filePath string) ([]gitdomain.ConflictHunk, error)

	StageFileFn         func(ctx context.Context, repoPath, filePath string) error
	StageHunkFn         func(ctx context.Context, repoPath, filePath, hunkID string) error
	UnstageFileFn       func(ctx context.Context, repoPath, filePath string) error
	UnstageHunkFn       func(ctx context.Context, repoPath, filePath, hunkID string) error
	DiscardFn           func(ctx context.Context, repoPath, filePath string) error
	CommitFn            func(ctx context.Context, repoPath, subject, body string) error
	PushFn              func(ctx context.Context, repoPath string) error
	FetchFn             func(ctx context.Context, repoPath string) error
	PullFn              func(ctx context.Context, repoPath, mode string) error
	CreateBranchFn      func(ctx context.Context, repoPath, name, source string, switchTo bool) error
	RenameBranchFn      func(ctx context.Context, repoPath, oldName, newName string) error
	DeleteBranchFn      func(ctx context.Context, repoPath, name string) error
	SwitchBranchFn      func(ctx context.Context, repoPath, name string) error
	StashPushFn         func(ctx context.Context, repoPath, message string) error
	StashApplyFn        func(ctx context.Context, repoPath, id string) error
	StashPopFn          func(ctx context.Context, repoPath, id string) error
	StashDropFn         func(ctx context.Context, repoPath, id string) error
	ResetFn             func(ctx context.Context, repoPath, mode, commit string) error
	MergeFn             func(ctx context.Context, repoPath, branch string) error
	RebaseFn            func(ctx context.Context, repoPath, onto string) error
	ResolveHunkFn       func(ctx context.Context, repoPath, filePath, hunkID string, resolution gitdomain.ConflictResolution, resolvedContent string) error
	OperationContinueFn func(ctx context.Context, repoPath string) error
	OperationAbortFn    func(ctx context.Context, repoPath string) error
}

// NewGitOpsEngine returns an empty GitOpsEngine.
func NewGitOpsEngine() *GitOpsEngine {
	return &GitOpsEngine{}
}

// AnyWriteOK installs no-op success closures for every mutating method.
func (g *GitOpsEngine) AnyWriteOK() {
	g.StageFileFn = func(context.Context, string, string) error { return nil }
	g.StageHunkFn = func(context.Context, string, string, string) error { return nil }
	g.UnstageFileFn = func(context.Context, string, string) error { return nil }
	g.UnstageHunkFn = func(context.Context, string, string, string) error { return nil }
	g.DiscardFn = func(context.Context, string, string) error { return nil }
	g.CommitFn = func(context.Context, string, string, string) error { return nil }
	g.PushFn = func(context.Context, string) error { return nil }
	g.FetchFn = func(context.Context, string) error { return nil }
	g.PullFn = func(context.Context, string, string) error { return nil }
	g.CreateBranchFn = func(context.Context, string, string, string, bool) error { return nil }
	g.RenameBranchFn = func(context.Context, string, string, string) error { return nil }
	g.DeleteBranchFn = func(context.Context, string, string) error { return nil }
	g.SwitchBranchFn = func(context.Context, string, string) error { return nil }
	g.StashPushFn = func(context.Context, string, string) error { return nil }
	g.StashApplyFn = func(context.Context, string, string) error { return nil }
	g.StashPopFn = func(context.Context, string, string) error { return nil }
	g.StashDropFn = func(context.Context, string, string) error { return nil }
	g.ResetFn = func(context.Context, string, string, string) error { return nil }
	g.MergeFn = func(context.Context, string, string) error { return nil }
	g.RebaseFn = func(context.Context, string, string) error { return nil }
	g.ResolveHunkFn = func(context.Context, string, string, string, gitdomain.ConflictResolution, string) error {
		return nil
	}
	g.OperationContinueFn = func(context.Context, string) error { return nil }
	g.OperationAbortFn = func(context.Context, string) error { return nil }
}

func (g *GitOpsEngine) Status(
	ctx context.Context,
	repoPath string,
) (gitdomain.GitStatus, error) {
	return g.StatusFn(ctx, repoPath)
}

func (g *GitOpsEngine) Diff(
	ctx context.Context,
	repoPath string,
	staged bool,
) ([]gitdomain.FileDiff, error) {
	return g.DiffFn(ctx, repoPath, staged)
}

func (g *GitOpsEngine) CommitDiff(
	ctx context.Context,
	repoPath string,
	sha string,
) (gitdomain.MultiFileDiff, error) {
	return g.CommitDiffFn(ctx, repoPath, sha)
}

func (g *GitOpsEngine) Log(
	ctx context.Context,
	repoPath string,
	limit int,
	skip int,
) ([]gitdomain.Commit, error) {
	return g.LogFn(ctx, repoPath, limit, skip)
}

func (g *GitOpsEngine) Blame(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]gitdomain.BlameEntry, error) {
	return g.BlameFn(ctx, repoPath, filePath)
}

func (g *GitOpsEngine) Branches(
	ctx context.Context,
	repoPath string,
) ([]gitdomain.Branch, error) {
	return g.BranchesFn(ctx, repoPath)
}

func (g *GitOpsEngine) Stashes(
	ctx context.Context,
	repoPath string,
) ([]gitdomain.Stash, error) {
	return g.StashesFn(ctx, repoPath)
}

func (g *GitOpsEngine) ConflictedFiles(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	return g.ConflictedFilesFn(ctx, repoPath)
}

func (g *GitOpsEngine) ConflictHunks(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]gitdomain.ConflictHunk, error) {
	return g.ConflictHunksFn(ctx, repoPath, filePath)
}

func (g *GitOpsEngine) StageFile(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	return g.StageFileFn(ctx, repoPath, filePath)
}

func (g *GitOpsEngine) StageHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
) error {
	return g.StageHunkFn(ctx, repoPath, filePath, hunkID)
}

func (g *GitOpsEngine) UnstageFile(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	return g.UnstageFileFn(ctx, repoPath, filePath)
}

func (g *GitOpsEngine) UnstageHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
) error {
	return g.UnstageHunkFn(ctx, repoPath, filePath, hunkID)
}

func (g *GitOpsEngine) Discard(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	return g.DiscardFn(ctx, repoPath, filePath)
}

func (g *GitOpsEngine) Commit(
	ctx context.Context,
	repoPath string,
	subject string,
	body string,
) error {
	return g.CommitFn(ctx, repoPath, subject, body)
}

func (g *GitOpsEngine) Push(
	ctx context.Context,
	repoPath string,
) error {
	return g.PushFn(ctx, repoPath)
}

func (g *GitOpsEngine) Fetch(
	ctx context.Context,
	repoPath string,
) error {
	return g.FetchFn(ctx, repoPath)
}

func (g *GitOpsEngine) Pull(
	ctx context.Context,
	repoPath string,
	mode string,
) error {
	return g.PullFn(ctx, repoPath, mode)
}

func (g *GitOpsEngine) CreateBranch(
	ctx context.Context,
	repoPath string,
	name string,
	source string,
	switchTo bool,
) error {
	return g.CreateBranchFn(ctx, repoPath, name, source, switchTo)
}

func (g *GitOpsEngine) RenameBranch(
	ctx context.Context,
	repoPath string,
	oldName string,
	newName string,
) error {
	return g.RenameBranchFn(ctx, repoPath, oldName, newName)
}

func (g *GitOpsEngine) DeleteBranch(
	ctx context.Context,
	repoPath string,
	name string,
) error {
	return g.DeleteBranchFn(ctx, repoPath, name)
}

func (g *GitOpsEngine) SwitchBranch(
	ctx context.Context,
	repoPath string,
	name string,
) error {
	return g.SwitchBranchFn(ctx, repoPath, name)
}

func (g *GitOpsEngine) StashPush(
	ctx context.Context,
	repoPath string,
	message string,
) error {
	return g.StashPushFn(ctx, repoPath, message)
}

func (g *GitOpsEngine) StashApply(
	ctx context.Context,
	repoPath string,
	id string,
) error {
	return g.StashApplyFn(ctx, repoPath, id)
}

func (g *GitOpsEngine) StashPop(
	ctx context.Context,
	repoPath string,
	id string,
) error {
	return g.StashPopFn(ctx, repoPath, id)
}

func (g *GitOpsEngine) StashDrop(
	ctx context.Context,
	repoPath string,
	id string,
) error {
	return g.StashDropFn(ctx, repoPath, id)
}

func (g *GitOpsEngine) Reset(
	ctx context.Context,
	repoPath string,
	mode string,
	commit string,
) error {
	return g.ResetFn(ctx, repoPath, mode, commit)
}

func (g *GitOpsEngine) Merge(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	return g.MergeFn(ctx, repoPath, branch)
}

func (g *GitOpsEngine) Rebase(
	ctx context.Context,
	repoPath string,
	onto string,
) error {
	return g.RebaseFn(ctx, repoPath, onto)
}

func (g *GitOpsEngine) ResolveHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
	resolution gitdomain.ConflictResolution,
	resolvedContent string,
) error {
	return g.ResolveHunkFn(ctx, repoPath, filePath, hunkID, resolution, resolvedContent)
}

func (g *GitOpsEngine) OperationContinue(
	ctx context.Context,
	repoPath string,
) error {
	return g.OperationContinueFn(ctx, repoPath)
}

func (g *GitOpsEngine) OperationAbort(
	ctx context.Context,
	repoPath string,
) error {
	return g.OperationAbortFn(ctx, repoPath)
}
