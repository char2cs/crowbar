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

// TestCreateChild_DerivesHumanReadableWorktreePath proves the managed child
// worktree lands at the human-readable
// <home>/projects/<project>/<slug>/<branch>/worktree path (spec §3.5/§3.9),
// with the slug resolved from the repo remote URL.
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

// TestRegression_CreateChild_DisambiguatesAFrozenName pins what replaced the
// refusal here.
//
// A workspace's directory is named for the branch it was CREATED on and never
// follows a later rename, so a name stays frozen by whoever was created under it
// — including after that workspace has moved to a different branch. Refusing
// would let a name the user has abandoned block them forever, so the create
// takes the next free variant instead.
//
// The sibling below differs only in case, which is the same directory on a
// case-insensitive filesystem: the disambiguation must catch that too.
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

// TestCreateChild_RemoteBranchAbsent_CreatesLocal verifies the spec-§3 decision:
// when the branch does NOT exist on the remote, CreateChild creates it locally
// from ParentBranch via WorktreeAddBranch and never fetches/checks out.
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
	// Child decision first: local remote-tracking ref (absent) then the live
	// remote query (absent) → create local. Only then is the parent start point
	// resolved (develop → not on remote → fork from the local ref).
	assert.Equal(t, []string{"RemoteTrackingBranchExists", "RemoteBranchExists", "RemoteBranchExists", "WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[3].args)
	// Fork point comes from the create-local startSha.
	assert.Equal(t, "localfork", created.ForkPointSha)
	assert.True(t, created.CreatedBranch, "a branch `worktree add -b` made is Crowbar's to delete later")
}

// TestCreateChild_RemoteBranchExists_ChecksOut verifies the spec-§3 decision:
// when the branch already exists on the remote, CreateChild refreshes
// origin/<branch> (FetchRef, which git never refuses) and checks the branch out
// AT that ref (WorktreeAddAtRef, i.e. `git worktree add -B`), so the worktree
// holds origin's commits even when a diverged local branch of the same name
// exists — NOT WorktreeAddBranch, and not a plain WorktreeAdd of the local ref.
// The fork point is the SHA that checkout resolved, so it is by construction the
// commit the worktree is on. The parent is never touched on this path: its start
// point is resolved past the checkout early-return, so the checkout pays no
// parent query and no parent fetch.
func TestCreateChild_RemoteBranchExists_ChecksOut(t *testing.T) {
	// Both the child branch AND the parent branch exist on the remote.
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
	// Op sequence: the child decision (local remote-tracking ref absent → falls
	// back to the live query, which says true), then fetch the child's remote ref,
	// read the local tip about to be reset, and check out AT origin/<branch>.
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists", // child local remote-tracking ref? → false
		"RemoteBranchExists",         // child exists live? → true
		"FetchRef",                   // refresh origin/<child>; never refused by git
		"RevParse",                   // local tip, read before the -B reset moves it
		"WorktreeAddAtRef",
		"SetUpstream", // link the checked-out branch to origin/<branch>
	}, g.ops())
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[1].args)
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[2].args)
	assert.Equal(t, []string{"/repo", "refs/heads/feature/x"}, g.calls[3].args)
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "origin/feature/x"}, g.calls[4].args,
		"the checkout must start AT origin's ref, not at whatever the local branch points to")
	assert.Equal(t, []string{"/repo", "feature/x"}, g.calls[5].args,
		"the imported branch is linked back to origin/feature/x")
	// WorktreeAddBranch must NOT be called on the checkout path, and neither may
	// a plain WorktreeAdd, which would check out the LOCAL ref.
	assert.NotContains(t, g.ops(), "WorktreeAddBranch")
	assert.NotContains(t, g.ops(), "WorktreeAdd")
	// The local branch ref must never be moved by a fetch on this path: `git fetch
	// origin <b>:<b>` is refused whenever <b> is checked out anywhere.
	assert.NotContains(t, g.ops(), "FastForwardBranch")
	assert.Equal(t, "remotefork", created.ForkPointSha)
	assert.False(t, created.CreatedBranch,
		"a local branch that already existed before the -B checkout stays the user's")
}

// TestCreateChild_NewBranch_ForksFromOriginParentTip is the direct regression for
// the field bug: a brand-new branch (not on the remote) whose parent IS on the
// remote must fork from ORIGIN's fresh parent tip, resolved via FetchRef +
// RevParse(origin/<parent>). It must NOT fast-forward the local parent ref
// (`git fetch origin <parent>:<parent>` is refused whenever the parent is checked
// out — which, for a protected parent, it permanently is in its locked managed
// worktree), and the resolved origin SHA — not the local parent name — must be
// handed to WorktreeAddBranch as the start point.
func TestCreateChild_NewBranch_ForksFromOriginParentTip(t *testing.T) {
	// Parent exists on remote; child does NOT — use per-branch map. RevParse of
	// origin/develop yields the fresh remote tip the child must fork from.
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
		"RemoteTrackingBranchExists", // child local remote-tracking ref? → false
		"RemoteBranchExists",         // child exists live? → false
		"RemoteBranchExists",         // parent exists on origin? → true
		"FetchRef",                   // fetch origin/develop (remote-tracking ref only)
		"RevParse",                   // resolve origin/develop's tip
		"WorktreeAddBranch",          // create the branch forked from that tip
	}, g.ops())
	assert.Equal(t, []string{"/repo", "my-feature"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "my-feature"}, g.calls[1].args)
	assert.Equal(t, []string{"/repo", "develop"}, g.calls[2].args)
	assert.Equal(t, []string{"/repo", "develop"}, g.calls[3].args)
	assert.Equal(t, []string{"/repo", "origin/develop"}, g.calls[4].args)
	// The new branch forks from the RESOLVED origin tip, not the stale local ref.
	assert.Equal(t, "origin-develop-tip", g.calls[5].args[3])
	// Never fast-forward the local parent ref — that hits git's checked-out refusal.
	assert.NotContains(t, g.ops(), "FastForwardBranch")
}

// TestCreateChild_ParentFetchFails_FallsBackToLocalTip proves a fetch failure on
// the parent (offline, dead remote) never blocks branch creation: the child is
// created from the LOCAL parent ref and the flow succeeds.
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
		"RemoteBranchExists", // child exists live? → false
		"RemoteBranchExists", // parent exists on origin? → true
		"FetchRef",           // …which fails
		"WorktreeAddBranch",  // still creates, from the local parent ref
	}, g.ops())
	// A failed fetch must not leave a resolved-tip attempt behind either.
	assert.NotContains(t, g.ops(), "RevParse")
	assert.Equal(t, []string{"/repo", created.WorktreePath, "my-feature", "develop"}, g.calls[4].args)
	assert.Equal(t, "localfork", created.ForkPointSha)
}

// TestCreateChild_LocalRemoteTrackingRef_ChecksOutDespiteLiveMiss is the unit
// encoding of the import bug fix: when a LOCAL remote-tracking ref for the branch
// exists, the checkout path is taken EVEN IF the live `ls-remote`
// (RemoteBranchExists) would answer false — the swallowed-live-failure that used
// to silently fork a same-named branch off the default branch. The live query is
// not even consulted for the child (short-circuit), and WorktreeAddBranch (the
// create-new-off-parent path) is never reached.
func TestCreateChild_LocalRemoteTrackingRef_ChecksOutDespiteLiveMiss(t *testing.T) {
	g := &fakeGit{
		trackingExistsByBranch: map[string]bool{"feature/x": true},
		remoteExists:           false, // live ls-remote spuriously says "not on remote"
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
	// The child decision resolves to checkout via the LOCAL tracking ref — the
	// live query is short-circuited, and the parent is never consulted at all.
	assert.Equal(t, []string{
		"RemoteTrackingBranchExists", // child local remote-tracking ref? → true
		"FetchRef",                   // refresh origin/<child>
		"RevParse",                   // local tip, read before the -B reset moves it
		"WorktreeAddAtRef",
		"SetUpstream", // link the checked-out branch to origin/<branch>
	}, g.ops())
	assert.NotContains(t, g.ops(), "WorktreeAddBranch",
		"a branch with a local remote-tracking ref must never be forked off the parent")
	assert.Equal(t, "remotefork", created.ForkPointSha)
}

// TestCreateChild_CheckoutBestEffortWhenFetchFailsButTrackingRefPresent proves
// the checkout is resilient: when the fetch (FetchRef) fails but the local
// origin/<branch> ref is present, the branch is still checked out AT that ref
// rather than aborting the import.
func TestCreateChild_CheckoutBestEffortWhenFetchFailsButTrackingRefPresent(t *testing.T) {
	g := &fakeGit{
		trackingExistsByBranch: map[string]bool{"feature/x": true},
		fetchRefErr:            errBoom, // offline fetch failure
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
		"RemoteTrackingBranchExists", // child local remote-tracking ref? → true → checkout
		"FetchRef",                   // fetch → fails
		"RemoteTrackingBranchExists", // re-check: local ref present → continue best-effort
		"RevParse",                   // local tip, read before the -B reset moves it
		"WorktreeAddAtRef",
		"SetUpstream", // link the checked-out branch to origin/<branch>
	}, g.ops())
	assert.Equal(t, "remotefork", created.ForkPointSha)
}

// TestCreateChild_RemoteTrackingBranchExistsError proves a genuine failure of the
// local remote-tracking check (a broken repo) is surfaced, not swallowed.
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
	// FetchRef failing on the CHILD branch (checkoutRemoteBranch path) is fatal
	// when there is no local origin/<branch> to fall back on: without either,
	// there is no remote content to import.
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
