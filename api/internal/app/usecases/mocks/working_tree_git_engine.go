package mocks

import (
	"context"

	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

// WorkingTreeGitEngine is a fake of the git WorkingTreeSummary surface.
type WorkingTreeGitEngine struct {
	WorkingTreeSummaryFn func(
		ctx context.Context,
		repoPath string,
		base string,
	) (int, int, bool, bool, error)
	// WouldMergeConflictsFn answers a batch; unset, every pair is clean.
	WouldMergeConflictsFn func(
		ctx context.Context,
		repoPath string,
		pairs []gitengine.MergePair,
	) []gitengine.MergeVerdict
	RevParseFn func(
		ctx context.Context,
		repoPath string,
		rev string,
	) (string, error)
}

// NewWorkingTreeGitEngine returns an empty WorkingTreeGitEngine.
func NewWorkingTreeGitEngine() *WorkingTreeGitEngine {
	return &WorkingTreeGitEngine{}
}

func (g *WorkingTreeGitEngine) WorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	base string,
) (int, int, bool, bool, error) {
	return g.WorkingTreeSummaryFn(ctx, repoPath, base)
}

func (g *WorkingTreeGitEngine) WouldMergeConflicts(
	ctx context.Context,
	repoPath string,
	pairs []gitengine.MergePair,
) []gitengine.MergeVerdict {
	if g.WouldMergeConflictsFn != nil {
		return g.WouldMergeConflictsFn(ctx, repoPath, pairs)
	}
	return make([]gitengine.MergeVerdict, len(pairs))
}

// RevParse resolves rev, or reports it as resolvable by default so summaryBase's
// verification step is a no-op unless a test opts into RevParseFn to exercise the
// fork-point fallback.
func (g *WorkingTreeGitEngine) RevParse(
	ctx context.Context,
	repoPath string,
	rev string,
) (string, error) {
	if g.RevParseFn == nil {
		return rev, nil
	}
	return g.RevParseFn(ctx, repoPath, rev)
}
