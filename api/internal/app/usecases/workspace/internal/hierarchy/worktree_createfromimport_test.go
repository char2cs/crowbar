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

// wsCaptureLastError wraps fakeWorkspace to record what CreateFromImport's
// placeholder fallback tells SetLastError, and to let a test make that write
// itself fail — fakeWorkspace's own SetLastError is a hardcoded no-op stub, so
// this is the only way to observe (or fail) that call.
type wsCaptureLastError struct {
	*fakeWorkspace
	gotID  string
	gotMsg string
	err    error
}

func (w *wsCaptureLastError) SetLastError(
	_ context.Context,
	id string,
	msg string,
) (domain.Workspace, error) {
	w.gotID, w.gotMsg = id, msg
	return domain.Workspace{}, w.err
}

// TestCreateFromImport_ProviderPRGraphError_FallsBackToDefaultParenting proves
// prBaseGraph's degrade path: a provider failure fetching open PRs must not
// abort the batch — the branch still imports, parented at the repo root under
// the default branch instead of under a PR base that can no longer be read.
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
		Branches: []string{"feat/x"},
	})
	require.NoError(t, err, "an open-PR graph failure must degrade, not abort the import")
	assert.Empty(t, created.ParentID,
		"with no PR graph to resolve a base, the branch parents at the repo root")
}

// TestCreateFromImport_ExistingWorkspaceListError_DegradesToFlatImport proves
// existingBranchWorkspaces' own degrade path: a List failure resolving already-
// imported branches must not abort the batch, only fall back to importing flat
// (no resolvable existing parent). The fake's List answers differently on its
// FIRST call (existingBranchWorkspaces, which fails) than on later calls (made
// from inside CreateChild's own guards, which must still succeed for the create
// to go through) — modelling exactly the best-effort contract CreateFromImport
// documents for this read.
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
		Branches: []string{"feat/x"},
	})
	require.NoError(t, err, "a failure resolving existing workspaces must degrade, not abort the import")
	assert.Empty(t, created.ParentID, "with no existing parent resolvable, the branch imports at the repo root")
}

// TestCreateFromImport_ParentsUnderExistingDefaultBranchWorkspace proves the
// positive side of existingBranchWorkspaces: an imported branch with no PR nests
// under the repo's ALREADY-EXISTING default-branch workspace rather than the
// repo root, once that lookup actually finds a match.
func TestCreateFromImport_ParentsUnderExistingDefaultBranchWorkspace(t *testing.T) {
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "ws-main", RepoID: "r1", Branch: "main", WorktreePath: "/repo"},
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
		Branches: []string{"feat/x"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ws-main", created.ParentID,
		"an imported branch with no PR nests under the existing default-branch workspace")
}

// TestCreateFromImport_BranchCreatedConcurrently_SkipsWithoutStranding proves
// createImportNode's dedup: when CreateChild itself reports ErrBranchWorkspaceExists
// (the branch got a workspace between the parent-resolution read and this create —
// exactly the race a 202'd batch can hit), the branch is treated as already
// represented, not as a batch failure. The fake's List disagrees with itself
// across calls to model that race: empty for the up-front existing-parent scan,
// then showing the new conflicting row by the time CreateChild checks.
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
		Branches: []string{"feat/x"},
	})
	require.NoError(t, err, "a branch that already has a workspace must not be reported as stranded")
	assert.Equal(t, 0, createCalls, "no new row (real or placeholder) is created for a branch that already has one")
}

// TestCreateFromImport_PlaceholderCreateAlsoFails_ReturnsStrandedError proves the
// one genuinely invisible outcome CreateFromImport's aggregated error exists for:
// a branch whose real create AND whose placeholder fallback both fail produces no
// row at all, and that branch is named in the returned error.
func TestCreateFromImport_PlaceholderCreateAlsoFails_ReturnsStrandedError(t *testing.T) {
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: []string{"feat/only"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "feat/only",
		"the aggregated error names every branch that produced no row at all")
}

// TestCreateFromImport_PlaceholderRecordsFailureCause proves that when a FREE
// branch (no live holder) still fails to materialise, the placeholder records
// WHY via SetLastError — the row's only way to explain itself when there is no
// HeldByPath to render instead.
func TestCreateFromImport_PlaceholderRecordsFailureCause(t *testing.T) {
	var placeholder workspace.CreateInput
	inner := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			placeholder = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	ws := &wsCaptureLastError{fakeWorkspace: inner}
	g := &fakeGit{addErr: errBoom} // WorktreeAddBranch fails: a plain git failure, no holder involved
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: []string{"feat/x"},
	})
	require.NoError(t, err, "a free branch that fails to materialise still yields a placeholder row")
	assert.Empty(t, placeholder.WorktreePath, "the placeholder signal is an empty worktree path")
	assert.Empty(t, placeholder.HeldByPath, "the branch was free, not held — there is no holder to name")
	assert.Equal(t, placeholder.ID, ws.gotID, "the recorded cause is attached to the placeholder row itself")
	assert.Contains(t, ws.gotMsg, "boom", "the cause names the actual git failure")
}

// TestCreateFromImport_PlaceholderCauseRecordFailure_IsBestEffort proves that a
// failure WRITING the cause (SetLastError itself erroring) never fails the
// import: the placeholder row already exists, which is what ends the client's
// spinner — the cause text is a nice-to-have on top of it.
func TestCreateFromImport_PlaceholderCauseRecordFailure_IsBestEffort(t *testing.T) {
	inner := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	ws := &wsCaptureLastError{fakeWorkspace: inner, err: errBoom}
	g := &fakeGit{addErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: []string{"feat/x"},
	})
	require.NoError(t, err, "a failed cause-write must not fail an import that already produced its row")
	assert.NotEmpty(t, ws.gotMsg, "the write was still attempted with the real cause")
}

// TestCreateFromImport_HolderPathUnresolvable_CrowbarHomeError proves
// resolveHolderPath degrades to "no holder to name" (rather than propagating)
// when crowbarHome itself cannot be resolved — the placeholder must still land,
// just without a Detach… target.
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
		Branches: []string{"feat/x"},
	})
	require.NoError(t, err, "an unresolvable crowbar home must still yield a placeholder row")
	assert.Empty(t, placeholder.HeldByPath, "with no crowbar home, there is no basis to name a holder")
}

// TestCreateFromImport_HolderPathUnresolvable_ResolveError mirrors the above for
// a genuine holder.Resolve failure (a broken `git worktree list`): the
// placeholder still lands with no holder path rather than losing the row.
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
		Branches: []string{"feat/x"},
	})
	require.NoError(t, err, "a broken holder resolution must still yield a placeholder row")
	assert.Empty(t, placeholder.HeldByPath, "an unresolvable holder is reported as no holder to name")
}
