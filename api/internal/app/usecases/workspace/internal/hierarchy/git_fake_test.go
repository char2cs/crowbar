package hierarchy_test

import (
	"context"
	"fmt"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

type gitCall struct {
	op   string
	args []string
}

type fakeGit struct {
	enginegit.Engine
	calls                  []gitCall
	addStartSha            string
	addErr                 error
	revParseSha            string
	revParseErr            error
	revParseByRev          map[string]string
	mergeBaseSha           string
	mergeBaseErr           error
	mergeErr               error
	squashErr              error
	rebaseErr              error
	ffErr                  error
	rebaseFFErr            error
	rebaseOnto             error
	operationAbortErr      error
	removeErr              error
	deleteErr              error
	remoteExists           bool
	remoteExistsByBranch   map[string]bool
	remoteExistsErr        error
	trackingExists         bool
	trackingExistsByBranch map[string]bool
	trackingExistsErr      error
	setUpstreamErr         error
	fetchRefErr            error
	fastForwardBranchErr   error
	worktreeAddErr         error

	summaryAdded        int
	summaryDeleted      int
	summaryHasCommits   bool
	summaryHasConflicts bool
	summaryErr          error
	pruneErr            error

	opInProgress    string
	opInProgressErr error

	worktrees              []enginegit.WorktreeEntry
	worktreeListErr        error
	detachErr              error
	checkoutErr            error
	addConflictUntilDetach error
	detachCalled           bool

	branchNow       string
	onBranchRenamed func()

	branches    []gitdomain.Branch
	branchesErr error

	uncommitted map[string]int
	unmerged    map[string]int
	riskErr     error
}

func (f *fakeGit) Branches(
	_ context.Context,
	repoPath string,
) ([]gitdomain.Branch, error) {
	f.record("Branches", repoPath)
	return f.branches, f.branchesErr
}

func (f *fakeGit) WorktreeList(
	_ context.Context,
	repoPath string,
) ([]enginegit.WorktreeEntry, error) {
	f.record("WorktreeList", repoPath)
	if f.detachCalled {
		return nil, f.worktreeListErr
	}
	return f.worktrees, f.worktreeListErr
}

func (f *fakeGit) DetachWorktree(
	_ context.Context,
	worktreePath string,
) error {
	f.record("DetachWorktree", worktreePath)
	f.detachCalled = true
	return f.detachErr
}

func (f *fakeGit) CheckoutBranch(
	_ context.Context,
	worktreePath string,
	branch string,
) error {
	f.record("CheckoutBranch", worktreePath, branch)
	return f.checkoutErr
}

func (f *fakeGit) OperationInProgress(
	_ context.Context,
	repoPath string,
) (string, error) {
	f.record("OperationInProgress", repoPath)
	return f.opInProgress, f.opInProgressErr
}

func (f *fakeGit) record(
	op string,
	args ...string,
) {
	f.calls = append(f.calls, gitCall{op: op, args: args})
}

func (f *fakeGit) ops() []string {
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, c.op)
	}
	return out
}

func (f *fakeGit) WorktreeAddBranch(
	_ context.Context,
	repoPath string,
	worktreePath string,
	branch string,
	startPoint string,
) (string, error) {
	f.record("WorktreeAddBranch", repoPath, worktreePath, branch, startPoint)
	return f.addStartSha, f.addErr
}

func (f *fakeGit) RemoteBranchExists(
	_ context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	f.record("RemoteBranchExists", repoPath, branch)
	if f.remoteExistsByBranch != nil {
		return f.remoteExistsByBranch[branch], f.remoteExistsErr
	}
	return f.remoteExists, f.remoteExistsErr
}

func (f *fakeGit) RemoteTrackingBranchExists(
	_ context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	f.record("RemoteTrackingBranchExists", repoPath, branch)
	if f.trackingExistsByBranch != nil {
		return f.trackingExistsByBranch[branch], f.trackingExistsErr
	}
	return f.trackingExists, f.trackingExistsErr
}

func (f *fakeGit) SetUpstream(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.record("SetUpstream", repoPath, branch)
	return f.setUpstreamErr
}

func (f *fakeGit) FetchRef(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.record("FetchRef", repoPath, branch)
	return f.fetchRefErr
}

func (f *fakeGit) FastForwardBranch(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.record("FastForwardBranch", repoPath, branch)
	return f.fastForwardBranchErr
}

func (f *fakeGit) WorktreeAdd(
	_ context.Context,
	repoPath string,
	worktreePath string,
	branch string,
) error {
	f.record("WorktreeAdd", repoPath, worktreePath, branch)
	if f.addConflictUntilDetach != nil && !f.detachCalled {
		return f.addConflictUntilDetach
	}
	return f.worktreeAddErr
}

func (f *fakeGit) WorktreeAddAtRef(
	_ context.Context,
	repoPath string,
	worktreePath string,
	branch string,
	startRef string,
) (string, error) {
	f.record("WorktreeAddAtRef", repoPath, worktreePath, branch, startRef)
	if f.addConflictUntilDetach != nil && !f.detachCalled {
		return "", f.addConflictUntilDetach
	}
	if f.worktreeAddErr != nil {
		return "", f.worktreeAddErr
	}
	return f.revParseSha, f.revParseErr
}

func (f *fakeGit) RevParse(
	_ context.Context,
	repoPath string,
	rev string,
) (string, error) {
	f.record("RevParse", repoPath, rev)
	if sha, ok := f.revParseByRev[rev]; ok {
		return sha, f.revParseErr
	}
	return f.revParseSha, f.revParseErr
}

func (f *fakeGit) MergeBase(
	_ context.Context,
	repoPath string,
	a string,
	b string,
) (string, error) {
	f.record("MergeBase", repoPath, a, b)
	return f.mergeBaseSha, f.mergeBaseErr
}

func (f *fakeGit) MergeCommit(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.record("MergeCommit", repoPath, branch)
	return f.mergeErr
}

func (f *fakeGit) MergeSquash(
	_ context.Context,
	repoPath string,
	branch string,
	subject string,
) error {
	f.record("MergeSquash", repoPath, branch, subject)
	return f.squashErr
}

func (f *fakeGit) Rebase(
	_ context.Context,
	repoPath string,
	onto string,
) error {
	f.record("Rebase", repoPath, onto)
	return f.rebaseErr
}

func (f *fakeGit) MergeFFOnly(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.record("MergeFFOnly", repoPath, branch)
	return f.ffErr
}

func (f *fakeGit) RebaseThenFFMerge(
	_ context.Context,
	childWorktree string,
	parentBranch string,
	parentWorktree string,
	childBranch string,
) error {
	f.record("RebaseThenFFMerge", childWorktree, parentBranch, parentWorktree, childBranch)
	return f.rebaseFFErr
}

func (f *fakeGit) RebaseOnto(
	_ context.Context,
	repoPath string,
	newTip string,
	forkPoint string,
	branch string,
) error {
	f.record("RebaseOnto", repoPath, newTip, forkPoint, branch)
	return f.rebaseOnto
}

func (f *fakeGit) OperationAbort(
	_ context.Context,
	repoPath string,
) error {
	f.record("OperationAbort", repoPath)
	return f.operationAbortErr
}

func (f *fakeGit) WorkingTreeSummary(
	_ context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	f.record("WorkingTreeSummary", repoPath, forkPointSha)
	return f.summaryAdded, f.summaryDeleted, f.summaryHasConflicts, f.summaryHasCommits, f.summaryErr
}

func (f *fakeGit) WorktreePrune(
	_ context.Context,
	repoPath string,
) error {
	f.record("WorktreePrune", repoPath)
	return f.pruneErr
}

func (f *fakeGit) WorktreeRemove(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	force bool,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.record("WorktreeRemove", repoPath, worktreePath, fmt.Sprint(force))
	return f.removeErr
}

func (f *fakeGit) UncommittedFiles(
	_ context.Context,
	worktreePath string,
) (int, error) {
	return f.uncommitted[worktreePath], f.riskErr
}

func (f *fakeGit) UnmergedCommits(
	_ context.Context,
	_ string,
	_ []string,
	dropBranch string,
) (int, error) {
	return f.unmerged[dropBranch], f.riskErr
}

func (f *fakeGit) ForceDeleteBranch(
	ctx context.Context,
	repoPath string,
	name string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.record("ForceDeleteBranch", repoPath, name)
	return f.deleteErr
}
