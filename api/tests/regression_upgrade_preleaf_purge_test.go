//go:build integration

package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type preLeafSibling struct {
	id     string
	branch string
	path   string
	locked bool
}

type preLeafInstall struct {
	home        string
	imported    importedRepo
	repo        domain.Repository
	slugDir     string
	siblings    map[string]preLeafSibling
	attachments []string
	journals    []string
}

var preLeafBranches = []string{"develop", "feature-a", "chats", "threads", "storages", "release"}

func seedPreLeafInstall(
	t *testing.T,
) *preLeafInstall {
	t.Helper()
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importProject(t, h)
	repoPtr, err := h.app.GORM.Repositories.FindByKey(context.Background(), imported.repoID)
	require.NoError(t, err)
	require.NotNil(t, repoPtr)
	repo := *repoPtr

	in := &preLeafInstall{
		home:     home,
		imported: imported,
		repo:     repo,
		slugDir:  filepath.Join(worktreepath.ProjectDir(home, imported.projectID), worktreepath.RemoteSlug(repo)),
		siblings: map[string]preLeafSibling{},
	}
	for _, branch := range preLeafBranches {
		path := filepath.Join(in.slugDir, branch)
		runGit(t, imported.repoPath, "worktree", "add", "-b", branch, path, "main")
		require.NoError(t, os.WriteFile(filepath.Join(path, "wip.txt"), []byte("uncommitted "+branch), 0o644))
		in.siblings[branch] = preLeafSibling{
			id: uuid.NewString(), branch: branch, path: path, locked: branch == "release",
		}
	}
	shared := filepath.Join(in.slugDir, "chats")
	for _, branch := range []string{"develop", "feature-a", "release"} {
		chatID := uuid.NewString()
		att := filepath.Join(shared, chatID, "attachments", "pic-"+branch+".png")
		require.NoError(t, os.MkdirAll(filepath.Dir(att), 0o755))
		require.NoError(t, os.WriteFile(att, []byte("png"), 0o644))
		in.attachments = append(in.attachments, att)
		journal := filepath.Join(worktreepath.LedgerChatsDir(home), chatID, "prompts", "r1.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(journal), 0o755))
		require.NoError(t, os.WriteFile(journal, []byte("{}"), 0o644))
		in.journals = append(in.journals, journal)
	}
	hook := filepath.Join(shared, ".hook-deliveries", "d1.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o755))
	require.NoError(t, os.WriteFile(hook, []byte("{}"), 0o644))
	in.attachments = append(in.attachments, hook)

	h.crash()
	seed := openBaseEra(t, home)
	now := time.Now().UTC()
	for _, s := range in.siblings {
		status := domain.WorkspaceStatus("")
		if s.locked {
			status = domain.WorkspaceStatusLocked
		}
		seed.workspace(domain.Workspace{
			ID: s.id, RepoID: imported.repoID, ProjectID: imported.projectID,
			Branch: s.branch, WorktreePath: s.path, ParentID: imported.workspaceID,
			Status: status, MergeStrategy: "merge", CreatedAt: now, LastActivity: now,
		})
	}
	seed.close()
	return in
}

func (in *preLeafInstall) assertUntouched(
	t *testing.T,
	keep ...string,
) {
	t.Helper()
	for _, branch := range keep {
		wip := filepath.Join(in.siblings[branch].path, "wip.txt")
		assert.FileExists(t, wip, "sibling %q lost its checkout or its uncommitted work", branch)
	}
	for _, f := range in.attachments {
		assert.FileExists(t, movedAttachment(in.home, f), "a file of the shared chats tree was removed")
	}
	for _, f := range in.journals {
		assert.FileExists(t, f, "a chat journal was removed")
	}
}

func TestRegression_Upgrade_PreLeafWorkspaceDeleteKeepsItsSiblings(t *testing.T) {
	in := seedPreLeafInstall(t)
	h := newHarnessAt(t, in.home)
	h.Quiesce()

	victim := in.siblings["develop"]
	row, err := h.app.Repositories.Workspace.Get(context.Background(), victim.id)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceProvisioned, row.Provisioning,
		"a pre-leaf row is a worktree Crowbar checked out")

	require.NoError(t, h.app.Usecases.Workspace.DeleteCascade(context.Background(), victim.id, domain.DiscardWorkAtRisk))
	h.QuiesceReactors()

	assert.NoDirExists(t, victim.path, "git removes the deleted workspace's own checkout")
	in.assertUntouched(t, "feature-a", "chats", "threads", "storages", "release")
	_, err = h.app.Repositories.Workspace.Get(context.Background(), victim.id)
	assert.Error(t, err, "the tombstone is purged")
}

func TestRegression_Upgrade_PreLeafRepoDeleteKeepsProtectedWorkAndOtherRepos(t *testing.T) {
	in := seedPreLeafInstall(t)
	h := newHarnessAt(t, in.home)
	h.Quiesce()
	otherPath := gitRepoWithCommit(t)
	runGit(t, otherPath, "remote", "add", "origin", "https://github.com/acme/other.git")
	addRepo(t, h, watchEntities(t, h, stream.KindRepo), in.imported.projectID, otherPath)
	h.Quiesce()
	otherWip := filepath.Join(worktreepath.ProjectDir(in.home, in.imported.projectID),
		"github.com/acme/other", "chats", "c1", "attachments", "a.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(otherWip), 0o755))
	require.NoError(t, os.WriteFile(otherWip, []byte("png"), 0o644))

	require.NoError(t, h.app.Usecases.ProjectDelete.DeleteRepo(context.Background(), in.repo, domain.DiscardWorkAtRisk))
	h.QuiesceReactors()

	for _, branch := range []string{"develop", "feature-a", "threads", "storages"} {
		assert.NoDirExists(t, in.siblings[branch].path, "git removes %s's checkout", branch)
	}
	assert.FileExists(t, filepath.Join(in.siblings["release"].path, "wip.txt"),
		"a protected worktree's uncommitted work survives its repo's delete")
	assert.FileExists(t, otherWip, "another repo's files survive")
	in.assertUntouched(t, "release", "chats")
}

func TestRegression_Upgrade_DeletingThePreLeafChatsBranchKeepsTheSharedTree(t *testing.T) {
	in := seedPreLeafInstall(t)
	h := newHarnessAt(t, in.home)
	h.Quiesce()

	require.NoError(t, h.app.Usecases.Workspace.DeleteCascade(context.Background(), in.siblings["chats"].id, domain.KeepWorkAtRisk))
	h.QuiesceReactors()

	in.assertUntouched(t, "develop", "feature-a", "threads", "storages", "release")
}

func TestRegression_Upgrade_PreLeafProjectDeleteKeepsOtherProjectsAndLiveCheckouts(t *testing.T) {
	in := seedPreLeafInstall(t)
	h := newHarnessAt(t, in.home)
	h.Quiesce()
	otherPath := gitRepoWithCommit(t)
	otherProject, otherRepo := createProjectAndRepo(t, h, otherPath)
	h.Quiesce()
	otherTree := filepath.Join(worktreepath.ProjectDir(in.home, otherProject), "slug", "chats", "c", "attachments", "a.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(otherTree), 0o755))
	require.NoError(t, os.WriteFile(otherTree, []byte("png"), 0o644))
	require.NotEmpty(t, otherRepo)

	deleteAccepted(t, h, "/v0/projects/"+in.imported.projectID, true)
	h.Quiesce()
	h.QuiesceReactors()

	assert.FileExists(t, otherTree, "another project's files survive")
	assert.FileExists(t, filepath.Join(in.siblings["release"].path, "wip.txt"),
		"a checkout git refused to remove keeps its uncommitted work")
}

func TestRegression_Upgrade_BootSweepOfAPreLeafTombstoneKeepsItsSiblings(t *testing.T) {
	in := seedPreLeafInstall(t)
	h := newHarnessAt(t, in.home)
	h.Quiesce()
	victim := in.siblings["develop"]
	row, err := h.app.Repositories.Workspace.Get(context.Background(), victim.id)
	require.NoError(t, err)
	require.Equal(t, domain.WorkspaceProvisioned, row.Provisioning)
	runGit(t, in.imported.repoPath, "worktree", "remove", "--force", victim.path)
	h.crash()
	seed := openBaseEra(t, in.home)
	seed.tombstone(victim.id)
	seed.close()

	h2 := newHarnessAt(t, in.home)
	h2.Quiesce()
	h2.QuiesceReactors()

	in.assertUntouched(t, "feature-a", "chats", "threads", "storages", "release")
	_, err = h2.app.Repositories.Workspace.Get(context.Background(), victim.id)
	assert.Error(t, err, "the sweep forgets the tombstone")
}
