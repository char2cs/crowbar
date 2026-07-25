package worktree_test

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
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// hiddenRemoteEngine wraps the REAL git engine but simulates the confirmed bug's
// trigger: a live `ls-remote` for a branch that spuriously fails (offline, auth,
// timeout, packaged-daemon PATH) and is swallowed as "not on the remote", plus
// the matching `fetch` failure — WHILE the local remote-tracking ref
// origin/<branch> is genuinely present (as `git branch -r` / the import list
// would show). Every other method delegates to the real engine driving real git.
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

// setupImportRepo builds a bare origin with a default `main` branch (content
// "MAIN") and a pushed `feature/x` branch (content "FEATURE-REAL"), then CLONES
// it. The clone mirrors the real import flow: origin/feature/x is present as a
// LOCAL remote-tracking ref (what `git branch -r` / the import panel enumerates),
// `main` is the only local branch, and there is NO local feature/x branch.
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

// TestRegression_ImportBranchChecksOutOriginContentNotParentFork reproduces the
// confirmed import bug: importing an existing origin branch silently created a
// NEW local branch off the DEFAULT branch (develop/main's content) instead of
// checking out origin's actual branch, because the checkout-vs-create decision
// gated only on a LIVE `ls-remote` that swallows any failure as false — while the
// import LIST that the user picks from reads the LOCAL remote-tracking refs.
//
// The scenario: origin/feature/x is present locally (from the clone), but the
// live remote query for feature/x fails (hiddenRemoteEngine returns false) and
// so does the fetch. With no parentId the import defaults ParentBranch to the
// repo default (main). BEFORE the fix the worktree lands on a fresh fork off main
// (content "MAIN"); AFTER the fix it must check out origin/feature/x
// (content "FEATURE-REAL"), tip == origin/feature/x, tracking origin/feature/x.
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
	uc := worktree.New(
		workspaces,
		engine,
		&stubProvider{},
		repos,
		func() time.Time { return time.Unix(1000, 0).UTC() },
		func() (string, error) { return t.TempDir(), nil },
	)

	// Import feature/x with NO parentId — exactly as the real import posts it, so
	// the create handler defaults ParentBranch to the repo default (main).
	child, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
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

// TestRegression_ImportBranchTracksOriginOnReachablePath is the ORIGIN-REACHABLE
// (happy-path) counterpart to the offline-fallback assertion above. It drives the
// REAL git engine with NO simulated failures, so the fetch succeeds: on that path
// FastForwardBranch (`git fetch origin <b>:<b>`) CREATES a local <b> with NO
// upstream, and `git worktree add <path> <b>` then checks out that already-existing
// local branch WITHOUT tracking — so, before the fix, the imported branch had
// origin's content but no `<b>@{upstream}`. The offline path (above) DWIMs tracking
// via `git worktree add`, so the two paths were inconsistent.
//
// An imported branch exists to be REVIEWED (branch-review + its PR vs its base), so
// it must be recognised as origin's <b>: checked out from origin's content AND
// linked back to origin (tracking origin/<b>). This asserts the upstream link on the
// reachable path — it FAILS before the fix ("fatal: no upstream configured") and
// passes after checkoutRemoteBranch sets it explicitly.
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

	// The REAL engine, origin reachable: the fetch succeeds and creates local
	// feature/x from origin/feature/x with no upstream — exactly the gap.
	uc := worktree.New(
		workspaces,
		enginegit.New(),
		&stubProvider{},
		repos,
		func() time.Time { return time.Unix(1000, 0).UTC() },
		func() (string, error) { return t.TempDir(), nil },
	)

	child, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
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

// setupImportChainRepo builds a bare origin with main → feat/base → feat/child,
// all pushed, then clones it. feat/base and feat/child are present only as
// origin remote-tracking refs (no local branches, no workspaces yet).
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

// TestRegression_ImportParentsPRChainCreatingMissingBase pins the production gap:
// importing a branch whose OPEN PR targets a base branch that is NOT yet a
// workspace must async-create that base (parented under the repo default) AND
// parent the imported branch under it — "parent the whole tree". Before this
// work, import forked every branch off the default with no PR parenting, so
// feat/child landed as a root under main and feat/base was never created.
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

	// Provider reports the open-PR graph: feat/child → feat/base → main.
	prov := &stubProvider{prLinks: []engineprovider.PRLink{
		{Head: "feat/child", Base: "feat/base"},
		{Head: "feat/base", Base: "main"},
	}}
	uc := worktree.New(
		workspaces,
		enginegit.New(),
		prov,
		repos,
		func() time.Time { return time.Unix(1000, 0).UTC() },
		func() (string, error) { return t.TempDir(), nil },
	)

	// Import ONLY feat/child — its missing base must be created and parented.
	err = uc.CreateFromImport(context.Background(), worktree.ImportInput{
		RepoID:        repoID,
		ProjectID:     projectID,
		RepoPath:      repoPath,
		RemoteURL:     "https://github.com/test/integration-repo.git",
		DefaultBranch: "main",
		Branches:      []string{"feat/child"},
	})
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

	// The created base carries origin/feat/base content, not a fork off main.
	got, err := os.ReadFile(filepath.Join(base.WorktreePath, "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, "BASE", string(got), "created base must check out origin/feat/base content")
}
