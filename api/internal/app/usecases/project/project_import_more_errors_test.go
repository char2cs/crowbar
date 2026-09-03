package project_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/defaultbranch"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

// TestImportRepo_IconWriteSkipped_WhenCrowbarHomeUnavailable proves writeRepoIcon
// degrades cleanly when CrowbarHome cannot be resolved: a local icon IS found on
// disk, but with nowhere under ~/.crowbar to write it the repo still imports,
// just with AvatarHasIcon left false (the generated label/color avatar).
func TestImportRepo_IconWriteSkipped_WhenCrowbarHomeUnavailable(t *testing.T) {
	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "favicon.svg"), []byte("<svg>icon</svg>"), 0o644))

	repos := mocks.NewRepositoryStore()
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	uc := project.NewImport(project.ImportDeps{
		Projects:    mocks.NewProjectStore(),
		Repos:       repos,
		Workspaces:  mocks.NewWorkspaceRepo(),
		Git:         git,
		Provider:    mocks.NewProviderEngine(),
		CrowbarHome: func() (string, error) { return "", errors.New("home boom") },
		Discover: func(root string, maxDepth int) ([]string, error) {
			return []string{repoDir}, nil
		},
		RefRunner: func(repoPath string) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) { return "", false }
		},
		Now:  func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat: statExists,
	})

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "an unresolvable crowbar home must not fail the import")
	require.Len(t, repos.Saved, 1)
	assert.False(t, repos.Saved[0].AvatarHasIcon,
		"the icon cannot be written with nowhere under crowbar home to put it")
}

// TestImport_ProtectedRowFailure_CleanupWorktreeRemoveAlsoFails mirrors
// TestImport_ProtectedRowFailure_CleansUpOrphanedWorktree for the failure case:
// when the best-effort cleanup of the orphaned worktree ALSO fails, that must be
// logged and swallowed, never surfaced as an import failure — the repo already
// has its home and stays imported either way.
func TestImport_ProtectedRowFailure_CleanupWorktreeRemoveAlsoFails(t *testing.T) {
	_, repos, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"main"}
	git.WorktreeRemoveErr = errors.New("remove boom")
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error) {
		if in.Protected {
			return domain.Workspace{}, errors.New("row boom")
		}
		created := domain.Workspace{ID: in.ID, Kind: in.Kind, IsDefault: in.IsDefault, WorktreePath: in.WorktreePath}
		ws.Created = append(ws.Created, created)
		return created, nil
	}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a failed best-effort worktree cleanup must not fail the import")
	require.Len(t, repos.Saved, 1, "the repo already has its home and is kept")
	require.Len(t, git.WorktreeAdds, 1, "the managed worktree was attempted")
}

// TestImport_RemoteProtectedBranch_WorktreeAddAtRefError_IsBestEffort proves a
// protected branch that exists on origin but fails its checkout AT origin's ref
// is skipped like any other per-branch provisioning failure: the repo (already
// holding its home) stays imported, and the failed branch simply gets no row.
func TestImport_RemoteProtectedBranch_WorktreeAddAtRefError_IsBestEffort(t *testing.T) {
	_, repos, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"release"}
	git.RemoteTrackingBranches = map[string]bool{"release": true}
	git.WorktreeAddErrByBranch = map[string]error{"release": errors.New("checkout boom")}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a failed protected-branch checkout is best-effort, not a batch failure")
	require.Len(t, repos.Saved, 1)
	for _, w := range ws.Created {
		assert.NotEqual(t, "release", w.Branch, "the branch whose checkout failed gets no row")
	}
}
