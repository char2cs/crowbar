package hierarchy_test

import (
	"context"
	"errors"
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
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

type hiddenRemoteEngine struct {
	enginegit.Engine
	hiddenBranches  map[string]bool
	failFetchBranch map[string]bool
}

func (h *hiddenRemoteEngine) RemoteBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	if h.hiddenBranches[branch] {
		return false, nil
	}
	return h.Engine.RemoteBranchExists(ctx, repoPath, branch)
}

func (h *hiddenRemoteEngine) FastForwardBranch(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	if h.failFetchBranch[branch] {
		return errors.New("simulated offline fetch failure")
	}
	return h.Engine.FastForwardBranch(ctx, repoPath, branch)
}

func setupImportRepo(
	t *testing.T,
) (string, string) {
	t.Helper()
	root := t.TempDir()

	origin := filepath.Join(root, "origin.git")
	gitRun(t, root, "init", "--bare", origin)

	seed := filepath.Join(root, "seed")
	require.NoError(t, os.MkdirAll(seed, 0o755))
	gitRun(t, seed, "init")
	gitRun(t, seed, "config", "user.email", "t@t")
	gitRun(t, seed, "config", "user.name", "t")
	writeFile(t, filepath.Join(seed, "f.txt"), "MAIN")
	gitRun(t, seed, "add", "f.txt")
	gitRun(t, seed, "commit", "-m", "main content")
	gitRun(t, seed, "branch", "-M", "main")
	gitRun(t, seed, "remote", "add", "origin", origin)
	gitRun(t, seed, "push", "origin", "main")
	gitRun(t, seed, "checkout", "-b", "feature/x")
	writeFile(t, filepath.Join(seed, "f.txt"), "FEATURE-REAL")
	gitRun(t, seed, "add", "f.txt")
	gitRun(t, seed, "commit", "-m", "feature content")
	gitRun(t, seed, "push", "origin", "feature/x")
	gitRun(t, origin, "symbolic-ref", "HEAD", "refs/heads/main")

	repoPath := filepath.Join(root, "repo")
	gitRun(t, root, "clone", origin, repoPath)
	return repoPath, "FEATURE-REAL"
}

func TestRegression_ImportBranchChecksOutOriginContentNotParentFork(t *testing.T) {
	repoPath, featureContent := setupImportRepo(t)

	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	workspaces, quiesce := newWorkspaceRepo(t, adapters)
	repos, err := storesqlite.NewFromDB[domain.Repository, string](adapters.GlobalView())
	require.NoError(t, err)

	repoID := "r1"
	projectID := "p1"
	require.NoError(t, repos.Save(context.Background(), domain.Repository{
		ID:            repoID,
		ProjectID:     projectID,
		Name:          "repo",
		Path:          repoPath,
		RemoteURL:     "https://github.com/test/integration-repo.git",
		DefaultBranch: "main",
	}))

	engine := &hiddenRemoteEngine{
		Engine:          enginegit.New(),
		hiddenBranches:  map[string]bool{"feature/x": true},
		failFetchBranch: map[string]bool{"feature/x": true},
	}
	uc := hierarchy.New(
		workspaces,
		engine,
		&stubProvider{},
		repos,
		func() time.Time { return time.Unix(1000, 0).UTC() },
		func() (string, error) { return t.TempDir(), nil },
	)
	withOwningChats(uc)

	child, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       repoID,
		ProjectID:    projectID,
		RepoPath:     repoPath,
		RemoteURL:    "https://github.com/test/integration-repo.git",
		Branch:       "feature/x",
		ParentID:     "",
		ParentBranch: "main",
	})
	require.NoError(t, err)
	quiesce()

	got, err := os.ReadFile(filepath.Join(child.WorktreePath, "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, featureContent, string(got),
		"imported worktree must carry origin/feature/x content, not a fresh fork off main")

	originTip := revParse(t, repoPath, "origin/feature/x")
	assert.Equal(t, originTip, revParse(t, child.WorktreePath, "HEAD"),
		"worktree tip must match origin/feature/x")
	assert.Equal(t, originTip, child.ForkPointSha,
		"fork point must be recorded as origin/feature/x's tip")

	upstream := trimNewline(gitRun(t, child.WorktreePath,
		"rev-parse", "--abbrev-ref", "feature/x@{upstream}"))
	assert.Equal(t, "origin/feature/x", upstream,
		"imported branch must track origin/feature/x (PR detection depends on it)")
}

func TestRegression_ImportBranchTracksOriginOnReachablePath(t *testing.T) {
	repoPath, featureContent := setupImportRepo(t)

	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	workspaces, quiesce := newWorkspaceRepo(t, adapters)
	repos, err := storesqlite.NewFromDB[domain.Repository, string](adapters.GlobalView())
	require.NoError(t, err)

	repoID := "r1"
	projectID := "p1"
	require.NoError(t, repos.Save(context.Background(), domain.Repository{
		ID:            repoID,
		ProjectID:     projectID,
		Name:          "repo",
		Path:          repoPath,
		RemoteURL:     "https://github.com/test/integration-repo.git",
		DefaultBranch: "main",
	}))

	uc := hierarchy.New(
		workspaces,
		enginegit.New(),
		&stubProvider{},
		repos,
		func() time.Time { return time.Unix(1000, 0).UTC() },
		func() (string, error) { return t.TempDir(), nil },
	)
	withOwningChats(uc)

	child, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       repoID,
		ProjectID:    projectID,
		RepoPath:     repoPath,
		RemoteURL:    "https://github.com/test/integration-repo.git",
		Branch:       "feature/x",
		ParentID:     "",
		ParentBranch: "main",
	})
	require.NoError(t, err)
	quiesce()

	got, err := os.ReadFile(filepath.Join(child.WorktreePath, "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, featureContent, string(got),
		"imported worktree must carry origin/feature/x content")

	originTip := revParse(t, repoPath, "origin/feature/x")
	assert.Equal(t, originTip, revParse(t, child.WorktreePath, "HEAD"),
		"worktree tip must match origin/feature/x")

	upstream := trimNewline(gitRun(t, child.WorktreePath,
		"rev-parse", "--abbrev-ref", "feature/x@{upstream}"))
	assert.Equal(t, "origin/feature/x", upstream,
		"origin-reachable import must ALSO track origin/feature/x — a proper review target")
}

func setupImportChainRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	origin := filepath.Join(root, "origin.git")
	gitRun(t, root, "init", "--bare", origin)

	seed := filepath.Join(root, "seed")
	require.NoError(t, os.MkdirAll(seed, 0o755))
	gitRun(t, seed, "init")
	gitRun(t, seed, "config", "user.email", "t@t")
	gitRun(t, seed, "config", "user.name", "t")
	writeFile(t, filepath.Join(seed, "f.txt"), "MAIN")
	gitRun(t, seed, "add", "f.txt")
	gitRun(t, seed, "commit", "-m", "main content")
	gitRun(t, seed, "branch", "-M", "main")
	gitRun(t, seed, "remote", "add", "origin", origin)
	gitRun(t, seed, "push", "origin", "main")
	gitRun(t, seed, "checkout", "-b", "feat/base")
	writeFile(t, filepath.Join(seed, "f.txt"), "BASE")
	gitRun(t, seed, "add", "f.txt")
	gitRun(t, seed, "commit", "-m", "base content")
	gitRun(t, seed, "push", "origin", "feat/base")
	gitRun(t, seed, "checkout", "-b", "feat/child")
	writeFile(t, filepath.Join(seed, "f.txt"), "CHILD")
	gitRun(t, seed, "add", "f.txt")
	gitRun(t, seed, "commit", "-m", "child content")
	gitRun(t, seed, "push", "origin", "feat/child")
	gitRun(t, origin, "symbolic-ref", "HEAD", "refs/heads/main")

	repoPath := filepath.Join(root, "repo")
	gitRun(t, root, "clone", origin, repoPath)
	return repoPath
}

func TestRegression_ImportParentsPRChainCreatingMissingBase(t *testing.T) {
	repoPath := setupImportChainRepo(t)

	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	workspaces, quiesce := newWorkspaceRepo(t, adapters)
	repos, err := storesqlite.NewFromDB[domain.Repository, string](adapters.GlobalView())
	require.NoError(t, err)

	repoID := "r1"
	projectID := "p1"
	require.NoError(t, repos.Save(context.Background(), domain.Repository{
		ID:            repoID,
		ProjectID:     projectID,
		Name:          "repo",
		Path:          repoPath,
		RemoteURL:     "https://github.com/test/integration-repo.git",
		DefaultBranch: "main",
	}))

	prov := &stubProvider{prLinks: []engineprovider.PRLink{
		{Head: "feat/child", Base: "feat/base"},
		{Head: "feat/base", Base: "main"},
	}}
	uc := hierarchy.New(
		workspaces,
		enginegit.New(),
		prov,
		repos,
		func() time.Time { return time.Unix(1000, 0).UTC() },
		func() (string, error) { return t.TempDir(), nil },
	)
	withOwningChats(uc)

	err = uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID:        repoID,
		ProjectID:     projectID,
		RepoPath:      repoPath,
		RemoteURL:     "https://github.com/test/integration-repo.git",
		DefaultBranch: "main",
		Branches:      branches("feat/child"),
	}, &outcomes{})
	require.NoError(t, err)
	quiesce()

	all, err := workspaces.List(context.Background())
	require.NoError(t, err)
	byBranch := map[string]domain.Workspace{}
	for _, w := range all {
		if w.RepoID == repoID && !w.IsDefault {
			byBranch[w.Branch] = w
		}
	}

	base, baseOK := byBranch["feat/base"]
	child, childOK := byBranch["feat/child"]
	require.True(t, baseOK, "missing PR base feat/base must be async-created as a workspace")
	require.True(t, childOK, "imported feat/child must be created")
	assert.Equal(t, "", base.ParentID, "feat/base's PR targets the default branch → root under repo home")
	assert.Equal(t, base.ID, child.ParentID, "feat/child must be parented under the feat/base workspace")

	got, err := os.ReadFile(filepath.Join(base.WorktreePath, "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, "BASE", string(got), "created base must check out origin/feat/base content")
}
