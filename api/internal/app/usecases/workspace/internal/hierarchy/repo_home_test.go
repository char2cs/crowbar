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

// TestCreateChild_AdoptMainWorktreeUnchanged proves the main-worktree adoption
// path (empty parent + branch == parent branch) is untouched by the spec-§3
// remote decision: it resolves HEAD directly and never queries the remote.
func TestCreateChild_AdoptMainWorktreeUnchanged(t *testing.T) {
	g := &fakeGit{revParseSha: "headsha"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(
			_ context.Context,
			in workspace.CreateInput,
			_ time.Time,
		) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
		// No workspace yet adopts the main worktree, so the first adoption proceeds.
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, nil },
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "main",
		ParentID:     "",
		ParentBranch: "main",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse"}, g.ops())
	assert.NotContains(t, g.ops(), "RemoteBranchExists")
	assert.Equal(t, "/repo", created.WorktreePath, "adopts the repo path as the worktree")
	assert.Equal(t, "headsha", created.ForkPointSha)
}

func TestCreateChild_AdoptMainWorktree_RevParseError(t *testing.T) {
	g := &fakeGit{revParseErr: errBoom}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, nil },
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoPath: "/repo", Branch: "main", ParentBranch: "main",
	})
	require.ErrorIs(t, err, errBoom)
}

// TestCreateChild_RejectsDuplicateBranch_AdoptPath proves a second workspace on
// the repo's default branch (empty parent + branch == default) is rejected with
// ErrBranchWorkspaceExists before any git work — no phantom duplicate row.
// TestCreateChild_DefaultWorkspaceDoesNotBlockImport pins the user-required rule:
// the default workspace (the imported repo folder) is unmanaged and must NOT
// count for the one-managed-workspace-per-branch guard — so it never blocks
// importing its own branch as a real managed workspace. With the default already
// adopting the repo path, creating develop again is allowed and goes to the
// managed-worktree path; it does NOT persist a second default.
func TestCreateChild_DefaultWorkspaceDoesNotBlockImport(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	var created workspace.CreateInput
	createCalls := 0
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "def", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true, Provisioning: domain.WorkspaceShared},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			createCalls++
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})

	require.NoError(t, err, "the default workspace must not block importing its branch")
	require.Equal(t, 1, createCalls)
	assert.False(t, created.IsDefault, "a repeat develop create must NOT adopt a second default")
	assert.NotEqual(t, "/repo", created.WorktreePath,
		"it goes to the managed-worktree path, not re-adopting the repo folder")
}

// A deleted prior adoption must NOT block re-adopting the main worktree (e.g.
// the default workspace was removed and is being recreated).
func TestCreateChild_AdoptMainWorktree_IgnoresDeletedAdoption(t *testing.T) {
	g := &fakeGit{revParseSha: "headsha"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "old", RepoID: "r1", Branch: "develop", Status: domain.WorkspaceStatusDeleted},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "develop", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, "/repo", created.WorktreePath)
}
