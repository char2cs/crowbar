package hierarchy_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestCreateChild_DerivesHumanReadableWorktreePath(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	home := t.TempDir()
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(),
		func() (string, error) { return home, nil })

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t,
		filepath.Join(home, "projects", "p1", "github.com", "test", "repo", "feature", "x", "worktree"),
		created.WorktreePath)
}

func TestRegression_CreateChild_DisambiguatesAFrozenName(t *testing.T) {
	home := t.TempDir()
	slugDir := filepath.Join(home, "projects", "p1", "github.com", "test", "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(slugDir, "Main"), 0o755))

	g := &fakeGit{addStartSha: "sha"}
	var got workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			got = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(),
		func() (string, error) { return home, nil })

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "main",
		ParentBranch: "develop",
	})

	require.NoError(t, err, "a frozen name must not block the create")
	assert.Equal(t, "main", got.Branch, "the BRANCH is still the one asked for")
	assert.Equal(t, filepath.Join(slugDir, "main-2", "worktree"), got.WorktreePath,
		"only the directory is disambiguated")
	assert.DirExists(t, filepath.Join(slugDir, "Main"), "the frozen sibling is untouched")
}

func TestCreateChild_RemoteBranchAbsent_CreatesLocal(t *testing.T) {
	g := &fakeGit{remoteExists: false, addStartSha: "localfork"}
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
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"RemoteTrackingBranchExists", "RemoteBranchExists", "RemoteBranchExists", "WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[3].args)
	assert.Equal(t, "localfork", created.ForkPointSha)
	assert.True(t, created.CreatedBranch, "a branch `worktree add -b` made is Crowbar's to delete later")
}

func TestCreateChild_RemoteBranchExists_ChecksOut(t *testing.T) {
	g := &fakeGit{remoteExists: true, revParseSha: "remotefork"}
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
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists",
		"RemoteBranchExists",
		"FetchRef",
		"RevParse",
		"WorktreeAddAtRef",
		"SetUpstream",
	}, g.ops())
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[1].args)
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[2].args)
	assert.Equal(t, []string{"/repo", "refs/heads/feature/x"}, g.calls[3].args)
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "origin/feature/x"}, g.calls[4].args,
		"the checkout must start AT origin's ref, not at whatever the local branch points to")
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[5].args,
		"the imported branch is linked back to origin/feature/x")
	assert.NotContains(t, g.ops(), "WorktreeAddBranch")
	assert.NotContains(t, g.ops(), "WorktreeAdd")
	assert.NotContains(t, g.ops(), "FastForwardBranch")
	assert.Equal(t, "remotefork", created.ForkPointSha)
	assert.False(t, created.CreatedBranch,
		"a local branch that already existed before the -B checkout stays the user's")
}

func TestCreateChild_NewBranch_ForksFromOriginParentTip(t *testing.T) {
	g := &fakeGit{
		remoteExistsByBranch: map[string]bool{"develop": true},
		revParseSha:          "origin-develop-tip",
		addStartSha:          "newsha",
	}
	uc := hierarchy.New(&fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "my-feature",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists",
		"RemoteBranchExists",
		"RemoteBranchExists",
		"FetchRef",
		"RevParse",
		"WorktreeAddBranch",
	}, g.ops())
	assert.Equal(t, []string{"/repo", "my-feature"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "my-feature"}, g.calls[1].args)
	assert.Equal(t, []string{"/repo", "develop"}, g.calls[2].args)
	assert.Equal(t, []string{"/repo", "develop"}, g.calls[3].args)
	assert.Equal(t, []string{"/repo", "origin/develop"}, g.calls[4].args)
	assert.Equal(t, "origin-develop-tip", g.calls[5].args[3])
	assert.NotContains(t, g.ops(), "FastForwardBranch")
}

func TestCreateChild_ParentFetchFails_FallsBackToLocalTip(t *testing.T) {
	g := &fakeGit{
		remoteExistsByBranch: map[string]bool{"develop": true},
		fetchRefErr:          errBoom,
		addStartSha:          "localfork",
	}
	var created workspace.CreateInput
	uc := hierarchy.New(&fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "my-feature",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists",
		"RemoteBranchExists",
		"RemoteBranchExists",
		"FetchRef",
		"WorktreeAddBranch",
	}, g.ops())
	assert.NotContains(t, g.ops(), "RevParse")
	assert.Equal(t, []string{"/repo", created.WorktreePath, "my-feature", "develop"}, g.calls[4].args)
	assert.Equal(t, "localfork", created.ForkPointSha)
}

func TestCreateChild_LocalRemoteTrackingRef_ChecksOutDespiteLiveMiss(t *testing.T) {
	g := &fakeGit{
		trackingExistsByBranch: map[string]bool{"feature/x": true},
		remoteExists:           false,
		revParseSha:            "remotefork",
	}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists",
		"FetchRef",
		"RevParse",
		"WorktreeAddAtRef",
		"SetUpstream",
	}, g.ops())
	assert.NotContains(t, g.ops(), "WorktreeAddBranch",
		"a branch with a local remote-tracking ref must never be forked off the parent")
	assert.Equal(t, "remotefork", created.ForkPointSha)
}

func TestCreateChild_CheckoutBestEffortWhenFetchFailsButTrackingRefPresent(t *testing.T) {
	g := &fakeGit{
		trackingExistsByBranch: map[string]bool{"feature/x": true},
		fetchRefErr:            errBoom,
		revParseSha:            "remotefork",
	}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err, "a fetch failure must be best-effort when origin/<branch> is present locally")
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists",
		"FetchRef",
		"RemoteTrackingBranchExists",
		"RevParse",
		"WorktreeAddAtRef",
		"SetUpstream",
	}, g.ops())
	assert.Equal(t, "remotefork", created.ForkPointSha)
}

func TestCreateChild_RemoteTrackingBranchExistsError(t *testing.T) {
	g := &fakeGit{trackingExistsErr: errBoom}
	uc := hierarchy.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoPath: "/r", ProjectID: "p1", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_RemoteBranchExistsError(t *testing.T) {
	g := &fakeGit{remoteExistsErr: errBoom}
	uc := hierarchy.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoPath: "/r", ProjectID: "p1", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_FetchRefError(t *testing.T) {
	g := &fakeGit{remoteExists: true, fetchRefErr: errBoom}
	uc := hierarchy.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoPath: "/r", ProjectID: "p1", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_CheckoutWorktreeAddError(t *testing.T) {
	g := &fakeGit{remoteExists: true, revParseSha: "s", worktreeAddErr: errBoom}
	uc := hierarchy.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoPath: "/r", ProjectID: "p1", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_CheckoutRevParseError(t *testing.T) {
	g := &fakeGit{remoteExists: true, revParseErr: errBoom}
	uc := hierarchy.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoPath: "/r", ProjectID: "p1", Branch: "b", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_WorktreeAddError(t *testing.T) {
	g := &fakeGit{addErr: errBoom}
	ws := &fakeWorkspace{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{RepoPath: "/r", ProjectID: "p1", RemoteURL: "https://github.com/test/repo.git", Branch: "b"})
	require.ErrorIs(t, err, errBoom)
}
