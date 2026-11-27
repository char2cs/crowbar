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

func samePathResolved(t *testing.T, want, got string) {
	t.Helper()
	wr, err := filepath.EvalSymlinks(want)
	require.NoError(t, err)
	gr, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	assert.Equal(t, wr, gr)
}

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

func TestRegression_ImportBranchHeldByExternalWorktreeYieldsPlaceholder(t *testing.T) {
	repoPath, _ := setupImportRepo(t)

	external := filepath.Join(t.TempDir(), "external-holder")
	gitRun(t, repoPath, "worktree", "add", external, "feature/x")

	uc, readAll := importHarness(t, repoPath)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID:        "r1",
		ProjectID:     "p1",
		RepoPath:      repoPath,
		RemoteURL:     "https://github.com/test/integration-repo.git",
		DefaultBranch: "main",
		Branches:      branches("feature/x"),
	}, &outcomes{})
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

func TestRegression_ImportFreeBranchStillProvisionsRealWorktree(t *testing.T) {
	repoPath, featureContent := setupImportRepo(t)

	uc, readAll := importHarness(t, repoPath)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID:        "r1",
		ProjectID:     "p1",
		RepoPath:      repoPath,
		RemoteURL:     "https://github.com/test/integration-repo.git",
		DefaultBranch: "main",
		Branches:      branches("feature/x"),
	}, &outcomes{})
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
