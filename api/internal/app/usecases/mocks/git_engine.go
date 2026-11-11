package mocks

import (
	"context"

	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

// GitEngine is a fake of the git operations the import usecase consumes.
type GitEngine struct {
	Worktrees       []gitengine.WorktreeEntry
	WorktreeListErr error
	MergeBaseSha    string
	MergeBaseErr    error

	// WorktreeListFn, when non-nil, overrides Worktrees/WorktreeListErr for
	// per-repo control in tests.
	WorktreeListFn func(repoPath string) ([]gitengine.WorktreeEntry, error)

	// Protected-branch managed-worktree provisioning fakes (project import).
	Detached          []string               // worktree paths detached to HEAD
	CheckedOut        []WorktreeAddCall      // (path, branch) re-attach calls
	WorktreeAdds      []WorktreeAddCall      // (path, branch) worktrees materialised, by EITHER add
	WorktreeAddAtRefs []WorktreeAddAtRefCall // the subset added AT a start ref (`git worktree add -B`)
	WorktreeRemoves   []string               // worktree paths removed
	// WorktreeRemovesUnforced are the removals asked for WITHOUT --force.
	WorktreeRemovesUnforced []string
	FetchedRefs             []string          // branches fetched from origin (FetchRef)
	FastForwardedBranches   []string          // branches fast-forwarded from origin (FastForwardBranch)
	RemoteBranches          map[string]bool   // branch -> exists on origin live (default false)
	RemoteTrackingBranches  map[string]bool   // branch -> local refs/remotes/origin/<branch> present (default false)
	MissingLocalBranches    map[string]bool   // branch -> no refs/heads/<branch> (default: every branch exists)
	RevParseShas            map[string]string // rev -> sha (default "")
	DetachErr               error             // forces DetachWorktree to fail
	// WorktreeAddErrByBranch forces WorktreeAdd to fail for specific branches.
	WorktreeAddErrByBranch map[string]error
	// Pruned records repo paths WorktreePrune was called on.
	Pruned []string
	// DeadRegistrations maps a dead worktree dir -> branch it "holds"; WorktreePrune
	// removes them and merges the survivors into the list holder.Resolve sees.
	DeadRegistrations map[string]string
	// FastForwardErr forces FastForwardBranch to fail (best-effort FF path).
	FastForwardErr error
	// UpstreamsSet records branches linked to origin/<branch> via SetUpstream.
	UpstreamsSet []string
	// SetUpstreamErr forces SetUpstream to fail (best-effort tracking path).
	SetUpstreamErr error
	// WorktreeRemoveErr forces WorktreeRemove to fail (orphaned-worktree cleanup
	// after a failed workspace-row create).
	WorktreeRemoveErr error
	// RevParseErr forces RevParse to fail (the non-essential fork-point read
	// after a protected-branch worktree add).
	RevParseErr error
	// FetchRefErr forces FetchRef to fail (best-effort origin refresh before
	// checking a protected branch out at its origin ref).
	FetchRefErr error
}

// WorktreeAddCall records a fake WorktreeAdd invocation.
type WorktreeAddCall struct {
	Path   string
	Branch string
}

// WorktreeAddAtRefCall records a `git worktree add -B <branch> <startRef>`, so a
// test can assert the checkout started at ORIGIN's ref rather than at whatever
// the local branch pointed to.
type WorktreeAddAtRefCall struct {
	Path     string
	Branch   string
	StartRef string
}

// NewGitEngine returns an empty GitEngine.
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

// SetUpstream records the branches linked back to origin/<branch>, so a test can
// assert the tracking info `git worktree add -B` does not create itself was set.
func (g *GitEngine) SetUpstream(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	g.UpstreamsSet = append(g.UpstreamsSet, branch)
	return g.SetUpstreamErr
}

// WorktreeAddAtRef shares WorktreeAdd's log and error map: to a caller the two are one outcome.
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
	g.DeadRegistrations = nil // prune reaps every dead registration
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

// RemoteTrackingBranchExists answers from RemoteTrackingBranches (default false).
func (g *GitEngine) RemoteTrackingBranchExists(_ context.Context, _, branch string) (bool, error) {
	return g.RemoteTrackingBranches[branch], nil
}

// LocalBranchExists answers from MissingLocalBranches (default: every branch exists).
func (g *GitEngine) LocalBranchExists(_ context.Context, _, branch string) (bool, error) {
	return !g.MissingLocalBranches[branch], nil
}
