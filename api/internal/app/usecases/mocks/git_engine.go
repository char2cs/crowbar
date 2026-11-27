package mocks

import (
	"context"

	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

type GitEngine struct {
	Worktrees       []gitengine.WorktreeEntry
	WorktreeListErr error
	MergeBaseSha    string
	MergeBaseErr    error

	WorktreeListFn func(repoPath string) ([]gitengine.WorktreeEntry, error)

	Detached                []string
	CheckedOut              []WorktreeAddCall
	WorktreeAdds            []WorktreeAddCall
	WorktreeAddAtRefs       []WorktreeAddAtRefCall
	WorktreeRemoves         []string
	WorktreeRemovesUnforced []string
	FetchedRefs             []string
	FastForwardedBranches   []string
	RemoteBranches          map[string]bool
	RemoteTrackingBranches  map[string]bool
	MissingLocalBranches    map[string]bool
	RevParseShas            map[string]string
	DetachErr               error
	WorktreeAddErrByBranch  map[string]error
	Pruned                  []string
	DeadRegistrations       map[string]string
	FastForwardErr          error
	UpstreamsSet            []string
	SetUpstreamErr          error
	WorktreeRemoveErr       error
	RevParseErr             error
	FetchRefErr             error
}

type WorktreeAddCall struct {
	Path   string
	Branch string
}

type WorktreeAddAtRefCall struct {
	Path     string
	Branch   string
	StartRef string
}

func NewGitEngine() *GitEngine {
	return &GitEngine{}
}

func (g *GitEngine) WorktreeList(
	ctx context.Context,
	repoPath string,
) ([]gitengine.WorktreeEntry, error) {
	if g.WorktreeListFn != nil {
		return g.WorktreeListFn(repoPath)
	}
	if g.WorktreeListErr != nil {
		return nil, g.WorktreeListErr
	}
	out := append([]gitengine.WorktreeEntry(nil), g.Worktrees...)
	for path, branch := range g.DeadRegistrations {
		out = append(out, gitengine.WorktreeEntry{Path: path, Branch: branch})
	}
	return out, nil
}

func (g *GitEngine) MergeBase(
	ctx context.Context,
	repoPath string,
	a string,
	b string,
) (string, error) {
	if g.MergeBaseErr != nil {
		return "", g.MergeBaseErr
	}
	return g.MergeBaseSha, nil
}

func (g *GitEngine) DetachWorktree(
	ctx context.Context,
	worktreePath string,
) error {
	if g.DetachErr != nil {
		return g.DetachErr
	}
	g.Detached = append(g.Detached, worktreePath)
	return nil
}

func (g *GitEngine) CheckoutBranch(
	ctx context.Context,
	worktreePath string,
	branch string,
) error {
	g.CheckedOut = append(g.CheckedOut, WorktreeAddCall{Path: worktreePath, Branch: branch})
	return nil
}

func (g *GitEngine) RemoteBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	return g.RemoteBranches[branch], nil
}

func (g *GitEngine) FetchRef(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	if g.FetchRefErr != nil {
		return g.FetchRefErr
	}
	g.FetchedRefs = append(g.FetchedRefs, branch)
	return nil
}

func (g *GitEngine) FastForwardBranch(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	if g.FastForwardErr != nil {
		return g.FastForwardErr
	}
	g.FastForwardedBranches = append(g.FastForwardedBranches, branch)
	return nil
}

func (g *GitEngine) WorktreeAdd(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
) error {
	if err := g.WorktreeAddErrByBranch[branch]; err != nil {
		return err
	}
	g.WorktreeAdds = append(g.WorktreeAdds, WorktreeAddCall{Path: worktreePath, Branch: branch})
	return nil
}

func (g *GitEngine) SetUpstream(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	g.UpstreamsSet = append(g.UpstreamsSet, branch)
	return g.SetUpstreamErr
}

func (g *GitEngine) WorktreeAddAtRef(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
	startRef string,
) (string, error) {
	if err := g.WorktreeAddErrByBranch[branch]; err != nil {
		return "", err
	}
	g.WorktreeAdds = append(g.WorktreeAdds, WorktreeAddCall{Path: worktreePath, Branch: branch})
	g.WorktreeAddAtRefs = append(g.WorktreeAddAtRefs, WorktreeAddAtRefCall{
		Path: worktreePath, Branch: branch, StartRef: startRef,
	})
	if sha, ok := g.RevParseShas[startRef]; ok {
		return sha, nil
	}
	return "", nil
}

func (g *GitEngine) WorktreeRemove(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	force bool,
) error {
	if !force {
		g.WorktreeRemovesUnforced = append(g.WorktreeRemovesUnforced, worktreePath)
	}
	if g.WorktreeRemoveErr != nil {
		return g.WorktreeRemoveErr
	}
	g.WorktreeRemoves = append(g.WorktreeRemoves, worktreePath)
	return nil
}

func (g *GitEngine) WorktreePrune(
	ctx context.Context,
	repoPath string,
) error {
	g.Pruned = append(g.Pruned, repoPath)
	g.DeadRegistrations = nil
	return nil
}

func (g *GitEngine) RevParse(
	ctx context.Context,
	repoPath string,
	rev string,
) (string, error) {
	if g.RevParseErr != nil {
		return "", g.RevParseErr
	}
	if sha, ok := g.RevParseShas[rev]; ok {
		return sha, nil
	}
	return "", nil
}

func (g *GitEngine) RemoteTrackingBranchExists(_ context.Context, _, branch string) (bool, error) {
	return g.RemoteTrackingBranches[branch], nil
}

func (g *GitEngine) LocalBranchExists(_ context.Context, _, branch string) (bool, error) {
	return !g.MissingLocalBranches[branch], nil
}
