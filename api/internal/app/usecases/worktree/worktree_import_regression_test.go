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
