package hierarchy_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// samePathResolved compares two paths with symlinks resolved: `git worktree
// list` emits fully-resolved paths (macOS /var -> /private/var), so a raw string
// compare against a t.TempDir() path is flaky by construction.
func samePathResolved(t *testing.T, want, got string) {
	t.Helper()
	wr, err := filepath.EvalSymlinks(want)
	require.NoError(t, err)
	gr, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	assert.Equal(t, wr, gr)
}

// importHarness wires the usecase against REAL git with a real read model, and
// returns the pieces an import scenario asserts over.
func importHarness(
	t *testing.T,
	repoPath string,
) (hierarchy.Usecase, func() (context.Context, []domain.Workspace)) {
	t.Helper()

	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	workspaces, quiesce := newWorkspaceRepo(t, adapters)
	repos, err := storesqlite.NewFromDB[domain.Repository, string](adapters.GlobalView())
	require.NoError(t, err)
	require.NoError(t, repos.Save(context.Background(), domain.Repository{
		ID:            "r1",
		ProjectID:     "p1",
		Name:          "repo",
		Path:          repoPath,
		RemoteURL:     "https://github.com/test/integration-repo.git",
		DefaultBranch: "main",
	}))

	crowbarHome := t.TempDir()
	uc := hierarchy.New(
		workspaces,
		enginegit.New(),
		&stubProvider{},
		repos,
		func() time.Time { return time.Unix(1000, 0).UTC() },
		func() (string, error) { return crowbarHome, nil },
	)
	withOwningChats(uc)
	readAll := func() (context.Context, []domain.Workspace) {
		quiesce()
		all, listErr := workspaces.List(context.Background())
		require.NoError(t, listErr)
		return context.Background(), all
	}
	return uc, readAll
}

func findByBranch(all []domain.Workspace, branch string) *domain.Workspace {
	for i := range all {
		if all[i].Branch == branch {
			return &all[i]
		}
	}
	return nil
}

// TestRegression_ImportBranchHeldByExternalWorktreeYieldsPlaceholder reproduces
// the confirmed production hang: importing a branch that a NON-Crowbar worktree
// already has checked out (e.g. another tool's worktree under ~/.superconductor)
// produced NOTHING — git refuses `worktree add` for an already-checked-out
// branch, CreateChild fails, CreateFromImport logs + `continue`s and returns nil,
// and the handler's broadcastLastError is a documented no-op for a blank wsID. No
// row ever reached the workspaces stream, so the FE's optimistic import row —
// which is cleared ONLY when a workspace with that branch arrives — spun forever
// with no toast.
//
// The fix routes this into the placeholder mechanism the protected-branch import
// already uses (spec §3.3): a row with an EMPTY WorktreePath carrying HeldByPath,
// which the FE renders as a placeholder with Retry / Detach… and toasts once.
// The row's mere existence is what ends the spinner.
func TestRegression_ImportBranchHeldByExternalWorktreeYieldsPlaceholder(t *testing.T) {
	repoPath, _ := setupImportRepo(t)

	// A live worktree OUTSIDE crowbar home holds feature/x — exactly the
	// production trigger (`fatal: 'feature/quiver-shell' is already used by
	// worktree at /Users/…/.superconductor/worktrees/…`).
	external := filepath.Join(t.TempDir(), "external-holder")
	gitRun(t, repoPath, "worktree", "add", external, "feature/x")

	uc, readAll := importHarness(t, repoPath)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID:        "r1",
		ProjectID:     "p1",
		RepoPath:      repoPath,
		RemoteURL:     "https://github.com/test/integration-repo.git",
		DefaultBranch: "main",
		Branches:      []string{"feature/x"},
	})
	require.NoError(t, err, "a held branch is a recoverable outcome, not a batch failure")

	_, all := readAll()
	got := findByBranch(all, "feature/x")
	require.NotNil(t, got,
		"import must produce a workspace row even when the branch is held; "+
			"without one the FE import spinner never resolves and nothing is toasted")
	assert.Empty(t, got.WorktreePath,
		"a held branch has no managed worktree — the empty path IS the placeholder signal")
	require.NotEmpty(t, got.HeldByPath,
		"the holder path is what the placeholder toast and the Detach… modal render")
	samePathResolved(t, external, got.HeldByPath)
	assert.NotEqual(t, domain.WorkspaceStatusLocked, got.Status,
		"an imported feature branch must not be locked; locked is for protected branches "+
			"and would survive provisioning to block merge/delete/rename forever")
}

// TestRegression_ImportFreeBranchStillProvisionsRealWorktree pins the happy path
// against the placeholder fallback: a branch nobody holds must still get a REAL
// managed worktree, never a placeholder. Without this, a fallback that fired too
// eagerly would turn every import into an unprovisioned row and look "fixed"
// because the spinner stopped.
func TestRegression_ImportFreeBranchStillProvisionsRealWorktree(t *testing.T) {
	repoPath, featureContent := setupImportRepo(t)

	uc, readAll := importHarness(t, repoPath)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID:        "r1",
		ProjectID:     "p1",
		RepoPath:      repoPath,
		RemoteURL:     "https://github.com/test/integration-repo.git",
		DefaultBranch: "main",
		Branches:      []string{"feature/x"},
	})
	require.NoError(t, err)

	_, all := readAll()
	got := findByBranch(all, "feature/x")
	require.NotNil(t, got)
	require.NotEmpty(t, got.WorktreePath, "a free branch must get a real managed worktree")
	assert.Empty(t, got.HeldByPath, "a provisioned workspace has no holder")
	content, readErr := os.ReadFile(filepath.Join(got.WorktreePath, "f.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, featureContent, string(content),
		"the worktree must carry origin's branch content")
}
