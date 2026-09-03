package realtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// revParseStubEngine controls RevParse independently of stubGitEngine (whose
// embedded enginegit.Engine leaves RevParse nil), so watcherDiffBase's own
// fallback branch can be driven without touching the Status-focused stub other
// tests in this package rely on.
type revParseStubEngine struct {
	enginegit.Engine
	err error
}

func (s revParseStubEngine) RevParse(
	_ context.Context,
	_ string,
	_ string,
) (string, error) {
	return "deadbeef", s.err
}

func TestWatcherDiffBase_NoParentNoBranch_FallsBackToForkPoint(t *testing.T) {
	ws := domain.Workspace{ForkPointSha: "fork-sha"}
	got := watcherDiffBase(context.Background(), stubWorkspaceRepo{}, revParseStubEngine{}, ws)
	assert.Equal(t, "fork-sha", got)
}

// TestWatcherDiffBase_OwnBranch_ResolvesViaRevParse covers a protected root's
// own branch: no parent to resolve, and RevParse confirms the branch still
// exists in the worktree, so the branch name itself is returned.
func TestWatcherDiffBase_OwnBranch_ResolvesViaRevParse(t *testing.T) {
	ws := domain.Workspace{Branch: "main", WorktreePath: "/repo", ForkPointSha: "fork-sha"}
	got := watcherDiffBase(context.Background(), stubWorkspaceRepo{}, revParseStubEngine{}, ws)
	assert.Equal(t, "main", got)
}

// TestWatcherDiffBase_OwnBranchUnresolvable_FallsBackToForkPoint covers a
// branch name that no longer resolves in the worktree (e.g. force-deleted),
// which must fall back to the recorded fork point rather than handing the
// watcher a ref git will keep failing to diff against.
func TestWatcherDiffBase_OwnBranchUnresolvable_FallsBackToForkPoint(t *testing.T) {
	ws := domain.Workspace{Branch: "deleted-branch", WorktreePath: "/repo", ForkPointSha: "fork-sha"}
	got := watcherDiffBase(context.Background(), stubWorkspaceRepo{}, revParseStubEngine{err: errors.New("unknown revision")}, ws)
	assert.Equal(t, "fork-sha", got)
}

// TestWatcherDiffBase_ChildWorkspace_UsesParentBranch covers a child
// workspace: the base to diff against is the PARENT's branch, not the child's
// own (a child has no independent "base" branch of its own).
func TestWatcherDiffBase_ChildWorkspace_UsesParentBranch(t *testing.T) {
	ws := domain.Workspace{ParentID: "parent-1", WorktreePath: "/repo", ForkPointSha: "fork-sha"}
	repo := stubWorkspaceRepo{ws: domain.Workspace{Branch: "develop"}}

	got := watcherDiffBase(context.Background(), repo, revParseStubEngine{}, ws)

	assert.Equal(t, "develop", got)
}

// TestWatcherDiffBase_ParentLookupFails_FallsBackToForkPoint covers an
// unresolvable parent row (e.g. the parent workspace was deleted): the watcher
// must degrade to the recorded fork point rather than propagating the error.
func TestWatcherDiffBase_ParentLookupFails_FallsBackToForkPoint(t *testing.T) {
	ws := domain.Workspace{ParentID: "gone", WorktreePath: "/repo", ForkPointSha: "fork-sha"}
	repo := stubWorkspaceRepo{err: errors.New("no such workspace")}

	got := watcherDiffBase(context.Background(), repo, revParseStubEngine{}, ws)

	assert.Equal(t, "fork-sha", got)
}
