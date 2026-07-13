package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
)

// fakeWorkspacePaths is a minimal wspaths.WorkspacePaths double: Get returns a
// preset path, Delete records that it was called. It lets bootSweepPurge run its
// fs teardown without a real view.db.
type fakeWorkspacePaths struct {
	path    string
	deleted bool
}

func (f *fakeWorkspacePaths) Put(_ context.Context, _ string, _ string) error { return nil }

func (f *fakeWorkspacePaths) Get(_ context.Context, _ string) (string, error) {
	return f.path, nil
}

func (f *fakeWorkspacePaths) Delete(_ context.Context, _ string) error {
	f.deleted = true
	return nil
}

var _ wspaths.WorkspacePaths = (*fakeWorkspacePaths)(nil)

// noopForgetDependents is the cascade callback for the fs-teardown tests that do not
// exercise it; the dependent-cascade behaviour has its own tests below.
func noopForgetDependents(_ context.Context, _ string) error { return nil }

// TestBootSweepPurge_RemovesWorkspaceRootIncludingSiblingChats is the parallel
// twin of repositories.TestWorktreeRemover_RemovesWorkspaceRootIncludingSiblingChatsDir:
// both the async delete reactor (worktreeRemover) and the crash-recovery boot
// orphan-sweep (bootSweepPurge) must rm the WHOLE workspace root — the "worktree"
// leaf plus the sibling "chats" tree — via filepath.Dir(path), not just the
// worktree leaf. This guards bootSweepPurge's parent-removal so a future edit to
// one site can't silently regress the other (spec §3.5/§3.8). The Forget of a
// non-existent aggregate returns ErrValidation, which bootSweepPurge swallows, so
// the fs teardown is exercised without needing a persisted workspace.
func TestBootSweepPurge_RemovesWorkspaceRootIncludingSiblingChats(t *testing.T) {
	c := newSweepContainer(t)
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo", "feat-x")
	worktreeLeaf := filepath.Join(root, "worktree")
	chatsDir := filepath.Join(root, "chats", "chatA", "ledger")

	// The "worktree" leaf was already cleared by git; only the sibling chats tree
	// remains on disk when the boot sweep re-drives the purge.
	require.NoError(t, os.MkdirAll(chatsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chatsDir, "00000001.turn"), []byte("{}"), 0o644))

	paths := &fakeWorkspacePaths{path: worktreeLeaf}
	purge := bootSweepPurge(c.axWorkspace, paths, home, noopForgetDependents)
	require.NoError(t, purge(context.Background(), "ws-does-not-exist"))

	_, err := os.Stat(root)
	assert.True(t, os.IsNotExist(err),
		"boot sweep must remove the whole workspace root (worktree + sibling chats)")
	assert.True(t, paths.deleted, "boot sweep must delete the id-path row")
}

// TestBootSweepPurge_RunsDependentCascade proves the boot sweep re-drives the
// dependent forget-cascade (review threads + agent chats), not just the worktree/row.
// A tombstone reaches the sweep precisely because its delete reactor never finished —
// it crashed, or the drain gate refused it at shutdown — so its chat aggregates are the
// thing still un-forgotten; the sweep is the only path that re-drives them.
func TestBootSweepPurge_RunsDependentCascade(t *testing.T) {
	c := newSweepContainer(t)
	home := t.TempDir()

	var cascaded string
	forget := func(_ context.Context, wsID string) error {
		cascaded = wsID
		return nil
	}
	paths := &fakeWorkspacePaths{path: ""}
	purge := bootSweepPurge(c.axWorkspace, paths, home, forget)
	require.NoError(t, purge(context.Background(), "ws-orphaned-chats"))

	assert.Equal(t, "ws-orphaned-chats", cascaded,
		"the boot sweep must re-drive the chat/thread forget-cascade for the swept workspace")
}

// TestBootSweepPurge_AbortsWhenCascadeFails proves the cascade's contract survives the
// boot path: if forgetting a dependent fails, the purge ABORTS — it does not rm the
// worktree or drop the id-path row — so the tombstone remains for the next boot to
// re-drive, rather than destroying the worktree with chats still dangling.
func TestBootSweepPurge_AbortsWhenCascadeFails(t *testing.T) {
	c := newSweepContainer(t)
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "ws")
	worktreeLeaf := filepath.Join(root, "worktree")
	require.NoError(t, os.MkdirAll(worktreeLeaf, 0o755))

	forget := func(_ context.Context, _ string) error {
		return assert.AnError
	}
	paths := &fakeWorkspacePaths{path: worktreeLeaf}
	purge := bootSweepPurge(c.axWorkspace, paths, home, forget)

	require.Error(t, purge(context.Background(), "ws-cascade-fails"),
		"a failed dependent cascade must abort the purge so the tombstone survives")
	_, err := os.Stat(worktreeLeaf)
	assert.NoError(t, err, "the worktree must NOT be removed when the cascade aborted")
	assert.False(t, paths.deleted, "the id-path row must NOT be dropped when the cascade aborted")
}

// TestBootSweepPurge_RefusesPathOutsideCrowbarHome guards the safety invariant
// (mirrors worktreeRemover's guard test): an adopted home/main worktree's mapped
// path is the user's REAL checkout outside the crowbar home and must never be
// rm'd, even after switching the removal target to the parent directory.
func TestBootSweepPurge_RefusesPathOutsideCrowbarHome(t *testing.T) {
	c := newSweepContainer(t)
	home := t.TempDir()
	outside := t.TempDir() // NOT under home
	userRepo := filepath.Join(outside, "user-repo")
	require.NoError(t, os.MkdirAll(userRepo, 0o755))

	paths := &fakeWorkspacePaths{path: userRepo}
	purge := bootSweepPurge(c.axWorkspace, paths, home, noopForgetDependents)
	require.NoError(t, purge(context.Background(), "ws-x"))

	_, err := os.Stat(userRepo)
	assert.NoError(t, err, "a path outside the crowbar home must never be removed")
}

// TestBootSweepPurge_HomeKindWorkspace_NeverRemovesRepoPath is the boot-sweep twin
// of the delete reactor's home-kind safety proof (Task 7): a crash-recovery
// re-drive of a tombstoned home-kind workspace must never rm the user's REAL
// repo.Path (outside crowbar home) or its parent.
func TestBootSweepPurge_HomeKindWorkspace_NeverRemovesRepoPath(t *testing.T) {
	c := newSweepContainer(t)
	home := t.TempDir()
	userRoot := t.TempDir()
	repoPath := filepath.Join(userRoot, "my-repo")
	sibling := filepath.Join(userRoot, "other-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(sibling, 0o755))

	paths := &fakeWorkspacePaths{path: repoPath}
	purge := bootSweepPurge(c.axWorkspace, paths, home, noopForgetDependents)
	require.NoError(t, purge(context.Background(), "ws-home"))

	_, err := os.Stat(repoPath)
	assert.NoError(t, err, "a home-kind workspace's repo.Path must never be removed by the boot sweep")
	_, err = os.Stat(sibling)
	assert.NoError(t, err, "repo.Path's PARENT (and siblings) must never be touched")
}

// TestBootSweepPurge_RefusesDegenerateLeafDirectlyUnderHome hardens the boot-sweep
// guard (Task 7): a residual path that is a direct child of home (filepath.Dir ==
// home) must never make the sweep rm crowbar home itself.
func TestBootSweepPurge_RefusesDegenerateLeafDirectlyUnderHome(t *testing.T) {
	c := newSweepContainer(t)
	home := t.TempDir()
	leaf := filepath.Join(home, "worktree") // filepath.Dir(leaf) == home
	require.NoError(t, os.MkdirAll(leaf, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "sentinel"), []byte("x"), 0o644))

	paths := &fakeWorkspacePaths{path: leaf}
	purge := bootSweepPurge(c.axWorkspace, paths, home, noopForgetDependents)
	require.NoError(t, purge(context.Background(), "ws-leaf"))

	_, err := os.Stat(home)
	assert.NoError(t, err, "crowbar home itself must never be removed by a degenerate leaf")
	_, err = os.Stat(filepath.Join(home, "sentinel"))
	assert.NoError(t, err, "home's other contents must survive")
}
