package worktree_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestCreateChild_DerivePathEscapesCrowbarHome_Rejected proves deriveWorktreePath
// surfaces worktreepath.Derive's escape guard: a repo whose remote does not parse
// and whose fallback slug (its NAME) carries path-traversal segments must not be
// allowed to land its worktree outside ~/.crowbar/projects.
func TestCreateChild_DerivePathEscapesCrowbarHome_Rejected(t *testing.T) {
	g := &fakeGit{}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			t.Fatal("the workspace row must never be created when the derived path escapes crowbar home")
			return domain.Workspace{}, nil
		},
	}
	// No remote URL and no PathSlug: RemoteSlug falls back to the repo NAME,
	// which here is a traversal payload.
	repos := &fakeRepoStore{name: "../../evil"}
	uc := worktree.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/repo", ProjectID: "p1", Branch: "feature/x", ParentBranch: "develop",
	})
	require.Error(t, err, "a slug that escapes crowbar home must be rejected before any git work")
	assert.Empty(t, g.calls, "no git runs against an unrepresentable path")
}

// TestCreateChild_RemoteBranchExists_LocalTipFastForwarded proves
// warnOnDiscardedLocalTip's fast-forward branch: when a diverging local branch
// tip turns out to be an ANCESTOR of origin's tip (merge-base == the local tip),
// nothing was actually lost, so the import proceeds without a stale-history
// warning. The fork point still comes from origin regardless.
func TestCreateChild_RemoteBranchExists_LocalTipFastForwarded(t *testing.T) {
	g := &fakeGit{
		remoteExists:  true,
		revParseSha:   "origintip",
		revParseByRev: map[string]string{"refs/heads/feature/x": "localtip"},
		mergeBaseSha:  "localtip", // base == localTip: a plain fast-forward
	}
	var forkPoint string
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			forkPoint = in.ForkPointSha
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/repo", ProjectID: "p1", RemoteURL: "https://github.com/test/repo.git",
		Branch: "feature/x", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Contains(t, g.ops(), "MergeBase",
		"a local tip that differs from origin's must be checked for divergence")
	for _, c := range g.calls {
		if c.op == "MergeBase" {
			assert.Equal(t, []string{"/repo", "localtip", "origintip"}, c.args,
				"compares the ACTUAL local tip against the ACTUAL resolved origin tip")
		}
	}
	assert.Equal(t, "origintip", forkPoint, "the worktree still resets to origin's tip regardless of the check")
}

// TestCreateChild_RemoteBranchExists_LocalTipDiverged_WarnsButSucceeds proves the
// other half: a local tip that is NOT an ancestor of origin's tip (real diverged
// history) only logs a diagnostic — it must never fail or alter the import, since
// the old commits stay reachable via the reflog.
func TestCreateChild_RemoteBranchExists_LocalTipDiverged_WarnsButSucceeds(t *testing.T) {
	g := &fakeGit{
		remoteExists:  true,
		revParseSha:   "origintip",
		revParseByRev: map[string]string{"refs/heads/feature/x": "localtip"},
		mergeBaseSha:  "some-other-common-ancestor", // base != localTip: genuinely diverged
	}
	var forkPoint string
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			forkPoint = in.ForkPointSha
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoPath: "/repo", ProjectID: "p1", RemoteURL: "https://github.com/test/repo.git",
		Branch: "feature/x", ParentBranch: "develop",
	})
	require.NoError(t, err, "diverged local history is diagnostic-only and must never block the import")
	assert.Equal(t, "origintip", forkPoint)
}
