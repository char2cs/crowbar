package hierarchy_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestCreateChild_DerivePathEscapesCrowbarHome_Rejected(t *testing.T) {
	g := &fakeGit{}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			t.Fatal("the workspace row must never be created when the derived path escapes crowbar home")
			return domain.Workspace{}, nil
		},
	}
	repos := &fakeRepoStore{name: "../../evil"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoPath: "/repo", ProjectID: "p1", Branch: "feature/x", ParentBranch: "develop",
	})
	require.Error(t, err, "a slug that escapes crowbar home must be rejected before any git work")
	assert.Empty(t, g.calls, "no git runs against an unrepresentable path")
}

func TestCreateChild_RemoteBranchExists_LocalTipFastForwarded(t *testing.T) {
	g := &fakeGit{
		remoteExists:  true,
		revParseSha:   "origintip",
		revParseByRev: map[string]string{"refs/heads/feature/x": "localtip"},
		mergeBaseSha:  "localtip",
	}
	var forkPoint string
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			forkPoint = in.ForkPointSha
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
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

func TestCreateChild_RemoteBranchExists_LocalTipDiverged_WarnsButSucceeds(t *testing.T) {
	g := &fakeGit{
		remoteExists:  true,
		revParseSha:   "origintip",
		revParseByRev: map[string]string{"refs/heads/feature/x": "localtip"},
		mergeBaseSha:  "some-other-common-ancestor",
	}
	var forkPoint string
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			forkPoint = in.ForkPointSha
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoPath: "/repo", ProjectID: "p1", RemoteURL: "https://github.com/test/repo.git",
		Branch: "feature/x", ParentBranch: "develop",
	})
	require.NoError(t, err, "diverged local history is diagnostic-only and must never block the import")
	assert.Equal(t, "origintip", forkPoint)
}
