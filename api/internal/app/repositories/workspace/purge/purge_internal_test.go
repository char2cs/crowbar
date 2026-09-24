package purge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorktreeRemover_RemovesWorkspaceRootIncludingSiblingChatsDir pins the
// workspace-root split (spec §3.5): the async delete reactor's rm -rf must
// remove the WHOLE workspace root — the "worktree" leaf `git worktree remove`
// already cleared plus the sibling "chats" tree it never touches — not just
// the worktree leaf itself. Without this, every agentic chat's ledger and
// segment tmp dirs would survive a workspace delete as an orphaned directory.
func TestWorktreeRemover_RemovesWorkspaceRootIncludingSiblingChatsDir(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo", "feat-x")
	worktreeLeaf := filepath.Join(root, "worktree")
	chatsDir := filepath.Join(root, "chats", "chatA", "ledger")

	// git worktree remove already cleared the "worktree" leaf by the time the
	// reactor's rm runs; only the sibling "chats" tree remains on disk.
	require.NoError(t, os.MkdirAll(chatsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chatsDir, "00000001.turn"), []byte("{}"), 0o644))

	// Finder leaves this behind the moment the root is opened; it must not be the
	// thing that keeps a deleted workspace's directory alive forever.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".DS_Store"), []byte("x"), 0o644))

	remove := WorktreeRemover(home)
	require.NoError(t, remove(worktreeLeaf))

	_, err := os.Stat(root)
	assert.True(t, os.IsNotExist(err), "the whole workspace root (worktree + sibling chats) must be gone")
}

// TestWorktreeRemover_RefusesPathOutsideCrowbarHome guards the pre-existing
// safety invariant: an adopted home/main worktree's mapped path (outside the
// crowbar home) is the user's REAL checkout and must never be rm -rf'd, even
// after switching the removal target to the parent directory.
func TestWorktreeRemover_RefusesPathOutsideCrowbarHome(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir() // a sibling temp dir, NOT under home
	userRepo := filepath.Join(outside, "user-repo")
	require.NoError(t, os.MkdirAll(userRepo, 0o755))

	remove := WorktreeRemover(home)
	require.NoError(t, remove(userRepo))

	_, err := os.Stat(userRepo)
	assert.NoError(t, err, "a path outside the crowbar home must never be removed")
}

// TestWorktreeRemover_HomeKindWorkspace_NeverRemovesRepoPath is the explicit
// home-kind safety proof (Task 7): a home-kind workspace's WorktreePath is the
// user's REAL repository (repo.Path, OUTSIDE crowbar home). The delete cascade
// must never rm that path OR its parent — doing so would delete the user's actual
// repo (and its siblings) off their disk. This pins the outside-home guard against
// a real repo.Path shape (with a .git dir and a sibling checkout), not a bare temp
// dir, so a regression that reroutes the rm can't slip past.
func TestWorktreeRemover_HomeKindWorkspace_NeverRemovesRepoPath(t *testing.T) {
	home := t.TempDir()
	userRoot := t.TempDir()                        // stands in for ~/code, OUTSIDE home
	repoPath := filepath.Join(userRoot, "my-repo") // the home-kind ws's WorktreePath
	sibling := filepath.Join(userRoot, "other-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(sibling, 0o755))

	remove := WorktreeRemover(home)
	require.NoError(t, remove(repoPath))

	_, err := os.Stat(repoPath)
	assert.NoError(t, err, "a home-kind workspace's repo.Path must never be removed")
	_, err = os.Stat(sibling)
	assert.NoError(t, err, "repo.Path's PARENT (and its siblings) must never be touched")
}

// TestWorktreeRemover_RefusesDegenerateLeafDirectlyUnderHome hardens the guard
// (Task 7): a path that is a direct child of home (so filepath.Dir(path) == home)
// must NOT cause the rm to delete crowbar home ITSELF. The removal target is the
// PARENT of the worktree leaf, so a one-segment path is refused — only a path with
// an intermediate segment below home has a parent still strictly under home.
func TestWorktreeRemover_RefusesDegenerateLeafDirectlyUnderHome(t *testing.T) {
	home := t.TempDir()
	leaf := filepath.Join(home, "worktree") // filepath.Dir(leaf) == home
	require.NoError(t, os.MkdirAll(leaf, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "sentinel"), []byte("x"), 0o644))

	remove := WorktreeRemover(home)
	require.NoError(t, remove(leaf))

	_, err := os.Stat(home)
	assert.NoError(t, err, "crowbar home itself must never be removed by a degenerate leaf")
	_, err = os.Stat(filepath.Join(home, "sentinel"))
	assert.NoError(t, err, "home's other contents must survive")
}

// Regression: a workspace on a NESTED branch (feature/x) roots a level deeper,
// so removing it left <slug>/feature behind — empty, invisible, and squatting
// the name. The next workspace on a branch actually called `feature` was then
// refused for a directory nothing occupied. The rename path already pruned this;
// delete did not, so the two ways a nested workspace can leave disagreed.
func TestRegression_WorktreeRemover_PrunesTheEmptiedNestedParent(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo")
	root := filepath.Join(slug, "feature", "x")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktree"), 0o755))

	remove := WorktreeRemover(home)
	require.NoError(t, remove(filepath.Join(root, "worktree")))

	assert.NoDirExists(t, filepath.Join(slug, "feature"),
		"the emptied nested parent must not squat the branch name")
}

// The prune can only take what it emptied: a nested parent another workspace
// still lives under stops the walk.
func TestWorktreeRemover_KeepsTheNestedParentASiblingStillOccupies(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo")
	sibling := filepath.Join(slug, "feature", "y", "worktree")
	require.NoError(t, os.MkdirAll(filepath.Join(slug, "feature", "x", "worktree"), 0o755))
	require.NoError(t, os.MkdirAll(sibling, 0o755))

	remove := WorktreeRemover(home)
	require.NoError(t, remove(filepath.Join(slug, "feature", "x", "worktree")))

	assert.DirExists(t, sibling, "a sibling under the same nested parent must survive")
	assert.DirExists(t, filepath.Join(slug, "feature"))
}

// The floor is <home>/projects/<projectID>: that level holds the project's icon,
// its `workspaces` state and its repo directories beside the slug trees, so the
// walk must never climb into it even when every slug beneath it is emptied.
func TestWorktreeRemover_PruneStopsAtTheProjectDirectory(t *testing.T) {
	home := t.TempDir()
	projectDir := filepath.Join(home, "projects", "p1")
	root := filepath.Join(projectDir, "github.com", "acme", "repo", "solo")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktree"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "workspaces"), 0o755))

	remove := WorktreeRemover(home)
	require.NoError(t, remove(filepath.Join(root, "worktree")))

	assert.DirExists(t, projectDir, "the project directory itself is never a candidate")
	assert.DirExists(t, filepath.Join(projectDir, "workspaces"), "project state must survive")
	assert.NoDirExists(t, filepath.Join(projectDir, "github.com"),
		"the emptied slug tree above the workspace is litter and goes")
}

// projectDirOf is the floor every walk in here leans on. It answers only for a
// path laid out the managed way, and refuses anything it cannot explain rather
// than handing back a directory a prune would then climb into.
func TestProjectDirOf_AnswersOnlyForTheManagedLayout(t *testing.T) {
	home := "/crow"

	dir, ok := projectDirOf("/crow/projects/p1/workspaces/w1", home)
	require.True(t, ok)
	assert.Equal(t, "/crow/projects/p1", dir)

	for _, path := range []string{
		"/crow/seed/harbour/infra", // an adopted checkout inside the home
		"/crow/projects",           // no project segment
		"/elsewhere/projects/p1/x", // not under the home at all
	} {
		_, ok := projectDirOf(path, home)
		assert.False(t, ok, "must refuse %q", path)
	}
}

// The prune only ever takes what it emptied, and never the floor.
func TestPruneEmptiedWorkspaceParents_StopsOnAnythingStillOccupied(t *testing.T) {
	home := t.TempDir()
	projectDir := filepath.Join(home, "projects", "p1")
	root := filepath.Join(projectDir, "github.com", "acme", "repo", "feature", "x")
	sibling := filepath.Join(projectDir, "github.com", "acme", "repo", "feature", "y")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(sibling, 0o755))
	require.NoError(t, os.RemoveAll(root))

	pruneEmptiedWorkspaceParents(root, home)

	assert.DirExists(t, sibling, "a sibling stops the walk dead")
	assert.DirExists(t, filepath.Join(projectDir, "github.com", "acme", "repo", "feature"))
}

// A path outside the managed layout is not walked at all.
func TestPruneEmptiedWorkspaceParents_IgnoresAnUnexplainablePath(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(home, "seed", "harbour", "infra")
	require.NoError(t, os.MkdirAll(outside, 0o755))

	pruneEmptiedWorkspaceParents(filepath.Join(outside, "gone"), home)

	assert.DirExists(t, outside, "an adopted checkout's parents are never candidates")
}

// TestRegression_WorktreeRemover_KeepsForeignSiblingsInTheWorkspaceRoot pins the
// limit of "the root IS the workspace's whole on-disk footprint".
//
// It is not, on a real machine: a <slug>/<branch> root was found holding five
// hand-made git worktrees beside the managed one, and the rm -rf of the root
// would have taken 4.5GB of checkouts Crowbar never created. Only the
// directories Crowbar makes may be removed; the root then survives precisely
// because something else is still in it.
func TestRegression_WorktreeRemover_KeepsForeignSiblingsInTheWorkspaceRoot(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo", "rewrite", "rust")
	worktreeLeaf := filepath.Join(root, "worktree")

	require.NoError(t, os.MkdirAll(filepath.Join(root, "chats", "chatA"), 0o755))
	// What `git worktree remove` left of the leaf: untracked litter, no .git link.
	require.NoError(t, os.MkdirAll(filepath.Join(worktreeLeaf, "node_modules"), 0o755))
	// A worktree someone else created beside the managed one, holding real work.
	foreign := filepath.Join(root, "wt-p3.31-dropdown")
	require.NoError(t, os.MkdirAll(filepath.Join(foreign, "src"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(foreign, "src", "main.rs"), []byte("fn main() {}"), 0o644))

	require.NoError(t, WorktreeRemover(home)(worktreeLeaf))

	assert.NoFileExists(t, filepath.Join(root, "chats", "chatA"),
		"crowbar's own chats tree must still be removed")
	assert.NoDirExists(t, worktreeLeaf, "the managed worktree must still be removed")
	assert.FileExists(t, filepath.Join(foreign, "src", "main.rs"),
		"a worktree crowbar did not create must survive the delete")
	assert.DirExists(t, root,
		"the root must survive while a foreign entry is still in it")
}

// A worktree leaf that still holds its `.git` link is a checkout git still has
// registered — one `git worktree remove` refused, like a protected worktree with
// uncommitted work (spec §3 P0-1). The purger must not rm -rf it: that would
// destroy the work and leave a dangling registration in the user's repository.
func TestRegression_WorktreeRemover_KeepsACheckoutGitStillRegisters(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "repo", "workspaces", "w1")
	worktreeLeaf := filepath.Join(root, "worktree")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "chats", "chatA"), 0o755))
	require.NoError(t, os.MkdirAll(worktreeLeaf, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeLeaf, ".git"), []byte("gitdir: /repo/.git/worktrees/w1"), 0o644))
	unsaved := filepath.Join(worktreeLeaf, "unsaved.txt")
	require.NoError(t, os.WriteFile(unsaved, []byte("work"), 0o644))

	require.NoError(t, WorktreeRemover(home)(worktreeLeaf))

	assert.FileExists(t, unsaved, "uncommitted work in a registered checkout survives")
	assert.NoDirExists(t, filepath.Join(root, "chats"), "crowbar's own chats tree still goes")
}
