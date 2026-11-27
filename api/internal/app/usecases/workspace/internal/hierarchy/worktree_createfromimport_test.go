package hierarchy_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

func TestCreateFromImport_ProviderPRGraphError_FallsBackToDefaultParenting(t *testing.T) {
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	g := &fakeGit{addStartSha: "sha"}
	prov := &fakeProvider{prErr: errBoom}
	uc := hierarchy.New(ws, g, prov, &fakeRepoStore{}, newNow(), fakeHome())
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: branches("feat/x"),
	}, &outcomes{})
	require.NoError(t, err, "an open-PR graph failure must degrade, not abort the import")
	assert.Empty(t, created.ParentID,
		"with no PR graph to resolve a base, the branch parents at the repo root")
}

func TestCreateFromImport_ExistingWorkspaceListError_DegradesToFlatImport(t *testing.T) {
	calls := 0
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			calls++
			if calls == 1 {
				return nil, errBoom
			}
			return nil, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	g := &fakeGit{addStartSha: "sha"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: branches("feat/x"),
	}, &outcomes{})
	require.NoError(t, err, "a failure resolving existing workspaces must degrade, not abort the import")
	assert.Empty(t, created.ParentID, "with no existing parent resolvable, the branch imports at the repo root")
}

func TestCreateFromImport_ParentsUnderExistingDefaultBranchWorkspace(t *testing.T) {
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "ws-main", RepoID: "r1", Branch: "main", WorktreePath: "/repo", Provisioning: domain.WorkspaceProvisioned},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	g := &fakeGit{addStartSha: "sha"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: branches("feat/x"),
	}, &outcomes{})
	require.NoError(t, err)
	assert.Equal(t, "ws-main", created.ParentID,
		"an imported branch with no PR nests under the existing default-branch workspace")
}

func TestCreateFromImport_ParentsUnderTheRepoHomeWhenTheDefaultBranchHasNoWorkspace(t *testing.T) {
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{
					ID: "ws-home", RepoID: "r1", Branch: "trunk", IsDefault: true, WorktreePath: "/repo",
					Provisioning: domain.WorkspaceShared,
				},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, &fakeGit{addStartSha: "sha"}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "trunk",
		Branches: branches("feat/x"),
	}, &outcomes{})
	require.NoError(t, err)
	assert.Equal(t, "ws-home", created.ParentID)
}

func TestCreateFromImport_BranchCreatedConcurrently_SkipsWithoutStranding(t *testing.T) {
	calls := 0
	createCalls := 0
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			calls++
			if calls == 1 {
				return nil, nil
			}
			return []domain.Workspace{{ID: "existing", RepoID: "r1", Branch: "feat/x"}}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			createCalls++
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: branches("feat/x"),
	}, &outcomes{})
	require.NoError(t, err, "a branch that already has a workspace must not be reported as stranded")
	assert.Equal(t, 0, createCalls, "no new row (real or placeholder) is created for a branch that already has one")
}

func TestCreateFromImport_HolderPathUnresolvable_CrowbarHomeError(t *testing.T) {
	var placeholder workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			placeholder = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	g := &fakeGit{addErr: errBoom}
	badHome := func() (string, error) { return "", errBoom }
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), badHome)
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: branches("feat/x"),
	}, &outcomes{})
	require.NoError(t, err, "an unresolvable crowbar home must still yield a placeholder row")
	assert.Empty(t, placeholder.HeldByPath, "with no crowbar home, there is no basis to name a holder")
}

func TestCreateFromImport_HolderPathUnresolvable_ResolveError(t *testing.T) {
	var placeholder workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			placeholder = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	g := &fakeGit{addErr: errBoom, worktreeListErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: branches("feat/x"),
	}, &outcomes{})
	require.NoError(t, err, "a broken holder resolution must still yield a placeholder row")
	assert.Empty(t, placeholder.HeldByPath, "an unresolvable holder is reported as no holder to name")
}

type outcomes struct {
	mu  sync.Mutex
	got []hierarchy.ImportOutcome
}

func (o *outcomes) Branch(x hierarchy.ImportOutcome) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.got = append(o.got, x)
}

func (o *outcomes) of(t *testing.T, branch string) hierarchy.ImportOutcome {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, x := range o.got {
		if x.Branch == branch {
			return x
		}
	}
	t.Fatalf("no outcome reported for %s (got %v)", branch, o.got)
	return hierarchy.ImportOutcome{}
}

func branches(names ...string) []hierarchy.ImportBranch {
	out := make([]hierarchy.ImportBranch, 0, len(names))
	for _, n := range names {
		out = append(out, hierarchy.ImportBranch{Name: n, ChatID: "chat-for-" + n})
	}
	return out
}

func importFixture(
	t *testing.T,
	g *fakeGit,
	fail map[string]error,
	links ...engineprovider.PRLink,
) (hierarchy.Usecase, *fakeOwningChats, *[]workspace.CreateInput) {
	t.Helper()
	var created []workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			if err := fail[in.Branch]; err != nil {
				return domain.Workspace{}, err
			}
			created = append(created, in)
			return domain.Workspace{ID: in.ID, Branch: in.Branch, WorktreePath: in.WorktreePath}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{prLinks: links}, &fakeRepoStore{}, newNow(), fakeHome())
	return uc, withOwningChats(uc), &created
}

func importInput(asks ...hierarchy.ImportBranch) hierarchy.ImportInput {
	return hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: asks,
	}
}

func importOf(names ...string) hierarchy.ImportInput { return importInput(branches(names...)...) }

func lockedAsk(name string) hierarchy.ImportBranch {
	return hierarchy.ImportBranch{Name: name, ChatID: "chat-for-" + name, Lock: true}
}

func protectedByBranch(created []workspace.CreateInput) map[string]bool {
	out := map[string]bool{}
	for _, in := range created {
		out[in.Branch] = in.Protected
	}
	return out
}

func TestCreateFromImport_ReportsEachBranchOutcome(t *testing.T) {
	uc, _, _ := importFixture(t, &fakeGit{addStartSha: "sha"}, nil)
	got := &outcomes{}

	require.NoError(t, uc.CreateFromImport(context.Background(), importOf("feat/a", "feat/b"), got))

	require.Len(t, got.got, 2)
	for _, branch := range []string{"feat/a", "feat/b"} {
		o := got.of(t, branch)
		require.NoError(t, o.Err)
		assert.NotEmpty(t, o.WorkspaceID)
	}
}

func TestCreateFromImport_UsesTheClientChatIDs(t *testing.T) {
	uc, chats, _ := importFixture(t, &fakeGit{addStartSha: "sha"}, nil)
	got := &outcomes{}

	require.NoError(t, uc.CreateFromImport(context.Background(), importOf("feat/a"), got))

	o := got.of(t, "feat/a")
	assert.Equal(t, "chat-for-feat/a", o.ChatID)
	owner, ok := chats.ownerOf(o.WorkspaceID)
	require.True(t, ok)
	assert.Equal(t, "chat-for-feat/a", owner, "the workspace is owned by the chat the client drew it as")
}

func TestCreateFromImport_AFailedBranchKeepsItsChatAsThePlaceholderOwner(t *testing.T) {
	uc, chats, created := importFixture(t, &fakeGit{addErr: errBoom}, nil)
	got := &outcomes{}

	require.NoError(t, uc.CreateFromImport(context.Background(), importOf("feat/a"), got))

	o := got.of(t, "feat/a")
	require.ErrorIs(t, o.Err, errBoom, "the outcome carries why the branch failed")
	require.Len(t, *created, 1)
	assert.Empty(t, (*created)[0].WorktreePath, "the row is a placeholder")
	assert.Equal(t, (*created)[0].ID, o.WorkspaceID)
	owner, ok := chats.ownerOf(o.WorkspaceID)
	require.True(t, ok)
	assert.Equal(t, "chat-for-feat/a", owner)
	assert.Equal(t, []string{"chat-for-feat/a"}, chats.minted, "one chat, minted once")
	assert.Empty(t, chats.discards(), "nothing is purged")
}

func TestCreateFromImport_AFailedPlaceholderStillReportsTheBranch(t *testing.T) {
	placeholderErr := errors.New("placeholder write down")
	uc, _, _ := importFixture(t, &fakeGit{addErr: errBoom}, map[string]error{"feat/a": placeholderErr})
	got := &outcomes{}

	require.NoError(t, uc.CreateFromImport(context.Background(), importOf("feat/a"), got))

	o := got.of(t, "feat/a")
	require.ErrorIs(t, o.Err, errBoom)
	require.ErrorIs(t, o.Err, placeholderErr, "both failures are named")
	assert.Empty(t, o.WorkspaceID)
}

func TestCreateFromImport_EachRequestedBranchTakesItsOwnLock(t *testing.T) {
	x := hierarchy.ImportBranch{Name: "x", ChatID: "chat-for-x"}
	for name, asks := range map[string][]hierarchy.ImportBranch{
		"child first":    {x, lockedAsk("y")},
		"ancestor first": {lockedAsk("y"), x},
	} {
		t.Run(name, func(t *testing.T) {
			uc, _, created := importFixture(t, &fakeGit{addStartSha: "sha"}, nil,
				engineprovider.PRLink{Head: "x", Base: "y"})

			require.NoError(t, uc.CreateFromImport(context.Background(), importInput(asks...), &outcomes{}))

			assert.Equal(t, map[string]bool{"x": false, "y": true}, protectedByBranch(*created),
				"created locked, not locked by a second command")
		})
	}
}

func TestCreateFromImport_AnAncestorNobodyAskedForStaysUnlocked(t *testing.T) {
	uc, _, created := importFixture(t, &fakeGit{addStartSha: "sha"}, nil,
		engineprovider.PRLink{Head: "x", Base: "y"})

	require.NoError(t, uc.CreateFromImport(context.Background(), importInput(lockedAsk("x")), &outcomes{}))

	assert.Equal(t, map[string]bool{"x": true, "y": false}, protectedByBranch(*created))
}
