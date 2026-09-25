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
)

// An install older than the "worktree" leaf recorded a workspace at
// <home>/projects/<P>/<slug>/<branch>: the checkout sat directly in the slug
// directory, and the chats tree every such workspace resolved (the sibling of
// its path) was ONE <slug>/chats shared by all of them. The pre-audit daemon
// carried a shape guard for exactly this; its removal made the purger treat the
// SLUG directory as one workspace's root, so deleting one workspace removed the
// shared chats tree and any sibling branch literally named chats, threads or
// storages.
//
// These tests seed that layout the way it exists on an upgrading user's disk —
// real git worktrees, uncommitted work, attachments and hook journals in the
// shared tree, rows written in the base version's shape — and then drive every
// delete path. Whatever is deleted, nothing that belongs to another workspace
// may go.

// preLeafSibling is one pre-leaf workspace of the seeded repo.
type preLeafSibling struct {
	id     string
	branch string
	path   string
	locked bool
}

// preLeafInstall is a crashed install holding pre-leaf workspaces of one repo.
type preLeafInstall struct {
	home     string
	imported importedRepo
	repo     domain.Repository
	slugDir  string
	siblings map[string]preLeafSibling // by branch
	// attachments and journals of chats that live in the shared tree.
	attachments []string
	journals    []string
}

// preLeafBranches are the pre-leaf workspaces seeded. develop and feature-a are
// ordinary siblings; chats, threads and storages are branches whose checkout
// sits exactly where a mis-rooted purge deletes; release is a protected branch
// with uncommitted work, which git itself refuses to remove.
var preLeafBranches = []string{"develop", "feature-a", "chats", "threads", "storages", "release"}

// seedPreLeafInstall imports a repo through the real flow, lays out pre-leaf
// worktrees and their shared chats tree on disk, crashes the daemon and writes
// the workspace rows in base's shape. The caller boots the new daemon.
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
	// The shared tree every pre-leaf row resolved as its chats dir: attachments
	// of chats in several siblings, the retired hook-delivery journal, runner
	// scratch. It is the same directory as the "chats" branch's checkout.
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

// assertUntouched checks everything the seed put on disk for the siblings in
// keep: their checkout with its uncommitted work, and every attachment, hook
// journal and prompt journal of the shared tree.
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
		assert.FileExists(t, f, "a file of the shared chats tree was removed")
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

	// Consent: the victim's own wip.txt is uncommitted work a plain delete keeps.
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
	addRepo(t, h, h.dial("/v0/projects/"+in.imported.projectID+"/repos"), in.imported.projectID, otherPath)
	h.Quiesce()
	otherWip := filepath.Join(worktreepath.ProjectDir(in.home, in.imported.projectID),
		"github.com/acme/other", "chats", "c1", "attachments", "a.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(otherWip), 0o755))
	require.NoError(t, os.WriteFile(otherWip, []byte("png"), 0o644))

	// Consent: every unlocked sibling holds an uncommitted wip.txt.
	require.NoError(t, h.app.Usecases.ProjectDelete.DeleteRepo(context.Background(), in.repo, domain.DiscardWorkAtRisk))
	h.QuiesceReactors()

	for _, branch := range []string{"develop", "feature-a", "threads", "storages"} {
		assert.NoDirExists(t, in.siblings[branch].path, "git removes %s's checkout", branch)
	}
	// git refuses a non-forced remove of a protected worktree with changes.
	assert.FileExists(t, filepath.Join(in.siblings["release"].path, "wip.txt"),
		"a protected worktree's uncommitted work survives its repo's delete")
	assert.FileExists(t, otherWip, "another repo's files survive")
	// The shared tree is not any one workspace's: the purge leaves it, and with
	// it the attachments of the protected worktree that outlives the repo. The
	// chats branch's checkout holds it, so git is not asked to force it away.
	in.assertUntouched(t, "release", "chats")
}

// The pre-leaf branch named chats is a checkout that also holds every
// sibling's chats tree. Deleting that workspace removes none of their files.
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
	// A second project, laid out the same way, must not lose a byte.
	otherPath := gitRepoWithCommit(t)
	otherProject, otherRepo := createProjectAndRepo(t, h, otherPath)
	h.Quiesce()
	otherTree := filepath.Join(worktreepath.ProjectDir(in.home, otherProject), "slug", "chats", "c", "attachments", "a.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(otherTree), 0o755))
	require.NoError(t, os.WriteFile(otherTree, []byte("png"), 0o644))
	require.NotEmpty(t, otherRepo)

	// Consent: every unlocked sibling holds an uncommitted wip.txt.
	deleteAccepted(t, h, "/v0/projects/"+in.imported.projectID, true)
	h.Quiesce()
	h.QuiesceReactors()

	assert.FileExists(t, otherTree, "another project's files survive")
	assert.FileExists(t, filepath.Join(in.siblings["release"].path, "wip.txt"),
		"a checkout git refused to remove keeps its uncommitted work")
}

// A delete a crash interrupted after its git half leaves a tombstone of an
// upgraded pre-leaf row; the next boot's sweep finishes it — and only it.
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
