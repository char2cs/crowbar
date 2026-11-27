package purge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestWorktreeRemover_RemovesWorkspaceRootIncludingSiblingChatsDir(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo", "feat-x")
	worktreeLeaf := filepath.Join(root, "worktree")
	chatsDir := filepath.Join(root, "chats", "chatA", "ledger")

	require.NoError(t, os.MkdirAll(chatsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chatsDir, "00000001.turn"), []byte("{}"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(root, ".DS_Store"), []byte("x"), 0o644))

	remove := removePath(home)
	require.NoError(t, remove(worktreeLeaf))

	_, err := os.Stat(root)
	assert.True(t, os.IsNotExist(err), "the whole workspace root (worktree + sibling chats) must be gone")
}

func TestWorktreeRemover_RefusesPathOutsideCrowbarHome(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	userRepo := filepath.Join(outside, "user-repo")
	require.NoError(t, os.MkdirAll(userRepo, 0o755))

	remove := removePath(home)
	require.NoError(t, remove(userRepo))

	_, err := os.Stat(userRepo)
	assert.NoError(t, err, "a path outside the crowbar home must never be removed")
}

func TestWorktreeRemover_HomeKindWorkspace_NeverRemovesRepoPath(t *testing.T) {
	home := t.TempDir()
	userRoot := t.TempDir()
	repoPath := filepath.Join(userRoot, "my-repo")
	sibling := filepath.Join(userRoot, "other-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(sibling, 0o755))

	remove := removePath(home)
	require.NoError(t, remove(repoPath))

	_, err := os.Stat(repoPath)
	require.NoError(t, err, "a home-kind workspace's repo.Path must never be removed")
	_, err = os.Stat(sibling)
	assert.NoError(t, err, "repo.Path's PARENT (and its siblings) must never be touched")
}

func TestWorktreeRemover_RefusesDegenerateLeafDirectlyUnderHome(t *testing.T) {
	home := t.TempDir()
	leaf := filepath.Join(home, "worktree")
	require.NoError(t, os.MkdirAll(leaf, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "sentinel"), []byte("x"), 0o644))

	remove := removePath(home)
	require.NoError(t, remove(leaf))

	_, err := os.Stat(home)
	require.NoError(t, err, "crowbar home itself must never be removed by a degenerate leaf")
	_, err = os.Stat(filepath.Join(home, "sentinel"))
	assert.NoError(t, err, "home's other contents must survive")
}

func TestRegression_WorktreeRemover_PrunesTheEmptiedNestedParent(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo")
	root := filepath.Join(slug, "feature", "x")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktree"), 0o755))

	remove := removePath(home)
	require.NoError(t, remove(filepath.Join(root, "worktree")))

	assert.NoDirExists(t, filepath.Join(slug, "feature"),
		"the emptied nested parent must not squat the branch name")
}

func TestWorktreeRemover_KeepsTheNestedParentASiblingStillOccupies(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo")
	sibling := filepath.Join(slug, "feature", "y", "worktree")
	require.NoError(t, os.MkdirAll(filepath.Join(slug, "feature", "x", "worktree"), 0o755))
	require.NoError(t, os.MkdirAll(sibling, 0o755))

	remove := removePath(home)
	require.NoError(t, remove(filepath.Join(slug, "feature", "x", "worktree")))

	assert.DirExists(t, sibling, "a sibling under the same nested parent must survive")
	assert.DirExists(t, filepath.Join(slug, "feature"))
}

func TestWorktreeRemover_PruneStopsAtTheProjectDirectory(t *testing.T) {
	home := t.TempDir()
	projectDir := filepath.Join(home, "projects", "p1")
	root := filepath.Join(projectDir, "github.com", "acme", "repo", "solo")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktree"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "workspaces"), 0o755))

	remove := removePath(home)
	require.NoError(t, remove(filepath.Join(root, "worktree")))

	assert.DirExists(t, projectDir, "the project directory itself is never a candidate")
	assert.DirExists(t, filepath.Join(projectDir, "workspaces"), "project state must survive")
	assert.NoDirExists(t, filepath.Join(projectDir, "github.com"),
		"the emptied slug tree above the workspace is litter and goes")
}

func TestProjectDirOf_AnswersOnlyForTheManagedLayout(t *testing.T) {
	home := "/crow"

	dir, ok := projectDirOf("/crow/projects/p1/workspaces/w1", home)
	require.True(t, ok)
	assert.Equal(t, "/crow/projects/p1", dir)

	for _, path := range []string{
		"/crow/seed/harbour/infra",
		"/crow/projects",
		"/elsewhere/projects/p1/x",
	} {
		_, ok := projectDirOf(path, home)
		assert.False(t, ok, "must refuse %q", path)
	}
}

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

func TestPruneEmptiedWorkspaceParents_IgnoresAnUnexplainablePath(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(home, "seed", "harbour", "infra")
	require.NoError(t, os.MkdirAll(outside, 0o755))

	pruneEmptiedWorkspaceParents(filepath.Join(outside, "gone"), home)

	assert.DirExists(t, outside, "an adopted checkout's parents are never candidates")
}

func TestRegression_WorktreeRemover_KeepsForeignSiblingsInTheWorkspaceRoot(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo", "rewrite", "rust")
	worktreeLeaf := filepath.Join(root, "worktree")

	require.NoError(t, os.MkdirAll(filepath.Join(root, "chats", "chatA"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(worktreeLeaf, "node_modules"), 0o755))
	foreign := filepath.Join(root, "wt-p3.31-dropdown")
	require.NoError(t, os.MkdirAll(filepath.Join(foreign, "src"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(foreign, "src", "main.rs"), []byte("fn main() {}"), 0o644))

	require.NoError(t, removePath(home)(worktreeLeaf))

	assert.NoFileExists(t, filepath.Join(root, "chats", "chatA"),
		"crowbar's own chats tree must still be removed")
	assert.NoDirExists(t, worktreeLeaf, "the managed worktree must still be removed")
	assert.FileExists(t, filepath.Join(foreign, "src", "main.rs"),
		"a worktree crowbar did not create must survive the delete")
	assert.DirExists(t, root,
		"the root must survive while a foreign entry is still in it")
}

func TestRegression_WorktreeRemover_KeepsACheckoutGitStillRegisters(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "repo", "workspaces", "w1")
	worktreeLeaf := filepath.Join(root, "worktree")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "chats", "chatA"), 0o755))
	require.NoError(t, os.MkdirAll(worktreeLeaf, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeLeaf, ".git"), []byte("gitdir: /repo/.git/worktrees/w1"), 0o644))
	unsaved := filepath.Join(worktreeLeaf, "unsaved.txt")
	require.NoError(t, os.WriteFile(unsaved, []byte("work"), 0o644))

	require.NoError(t, removePath(home)(worktreeLeaf))

	assert.FileExists(t, unsaved, "uncommitted work in a registered checkout survives")
	assert.NoDirExists(t, filepath.Join(root, "chats"), "crowbar's own chats tree still goes")
}

func removePath(home string) func(path string) error {
	remove := WorktreeRemover(home, noRows)
	return func(path string) error {
		return remove(context.Background(), domain.Workspace{ID: "victim", WorktreePath: path})
	}
}

func noRows(context.Context) ([]domain.Workspace, error) { return nil, nil }

func TestWorktreeRemover_RefusesAPreLeafPathAndKeepsEverySibling(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "P", "github.com", "acme", "app")
	victim := filepath.Join(slug, "develop")
	kept := []string{
		filepath.Join(slug, "chats", "wip.txt"),
		filepath.Join(slug, "chats", "c1", "attachments", "a.png"),
		filepath.Join(slug, "threads", "wip.txt"),
		filepath.Join(slug, "storages", "wip.txt"),
		filepath.Join(slug, "main", "worktree", "wip.txt"),
		filepath.Join(victim, "left-by-git.txt"),
	}
	for _, f := range kept {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	}

	require.NoError(t, removePath(home)(victim))

	for _, f := range kept {
		assert.FileExists(t, f)
	}
}

func TestWorktreeRemover_RemovesAnIdentityKeyedRoot(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "P", "workspaces", "W1")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktree"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "chats"), 0o755))

	require.NoError(t, removePath(home)(filepath.Join(root, "worktree")))

	assert.NoDirExists(t, root, "the whole workspace root is the footprint")
}

func TestWorktreeRemover_KeepsARootAnotherRowClaims(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "P", "app")
	shared := filepath.Join(slug, "chats")
	attachment := filepath.Join(shared, "chats", "c1", "a.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(attachment), 0o755))
	require.NoError(t, os.WriteFile(attachment, []byte("x"), 0o644))
	rows := func(context.Context) ([]domain.Workspace, error) {
		return []domain.Workspace{{ID: "sibling", WorktreePath: filepath.Join(slug, "develop")}}, nil
	}

	remove := WorktreeRemover(home, rows)
	require.NoError(t, remove(context.Background(),
		domain.Workspace{ID: "victim", WorktreePath: filepath.Join(shared, "worktree")}))

	assert.FileExists(t, attachment)
}

func TestWorktreeRemover_KeepsARootReachedThroughASymlink(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "projects", "P", "workspaces", "W1")
	chat := filepath.Join(target, "chats", "c1")
	require.NoError(t, os.MkdirAll(chat, 0o755))
	alias := filepath.Join(home, "projects", "P", "workspaces", "alias")
	require.NoError(t, os.Symlink(target, alias))

	require.NoError(t, removePath(home)(filepath.Join(alias, "worktree")))

	assert.DirExists(t, chat)
}

func TestWorktreeRemover_ARowListingFailureRemovesNothing(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "P", "workspaces", "W1")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "chats"), 0o755))
	failing := func(context.Context) ([]domain.Workspace, error) { return nil, errors.New("db closed") }

	err := WorktreeRemover(home, failing)(context.Background(),
		domain.Workspace{ID: "w1", WorktreePath: filepath.Join(root, "worktree")})

	require.Error(t, err)
	assert.DirExists(t, filepath.Join(root, "chats"))
}

type purgeFixture struct {
	home, outside string
	rows          []domain.Workspace
	ownRoots      map[string]string
}

func newPurgeFixture(t *testing.T) purgeFixture {
	t.Helper()
	home, outside := t.TempDir(), t.TempDir()
	project := filepath.Join(home, "projects", "P")
	slug := filepath.Join(project, "github.com", "acme", "app")
	identity := filepath.Join(project, "r1", "workspaces", "W1")
	files := []string{
		filepath.Join(slug, "main", "worktree", ".git"),
		filepath.Join(slug, "main", "worktree", "wip.txt"),
		filepath.Join(slug, "main", "chats", "cM", "a.png"),
		filepath.Join(identity, "worktree", "untracked.txt"),
		filepath.Join(identity, "chats", "cW", "a.png"),
		filepath.Join(identity, "storages", "s.db"),
		filepath.Join(slug, "develop", ".git"),
		filepath.Join(slug, "develop", "wip.txt"),
		filepath.Join(slug, "chats", ".git"),
		filepath.Join(slug, "chats", "wip.txt"),
		filepath.Join(slug, "chats", "cX", "attachments", "p.png"),
		filepath.Join(slug, "chats", ".hook-deliveries", "d.json"),
		filepath.Join(slug, "threads", ".git"),
		filepath.Join(slug, "threads", "wip.txt"),
		filepath.Join(slug, "storages", ".git"),
		filepath.Join(slug, "storages", "wip.txt"),
		filepath.Join(home, "chats", "c1", "prompts", "r.json"),
		filepath.Join(project, "storages", "s.db"),
		filepath.Join(outside, "victim", "worktree", "f"),
		filepath.Join(outside, "victim", "chats", "f"),
	}
	for _, f := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	}
	require.NoError(t, os.Symlink(filepath.Join(slug, "main"), filepath.Join(slug, "alias")))
	rows := []domain.Workspace{
		{ID: "main", WorktreePath: filepath.Join(slug, "main", "worktree")},
		{ID: "w1", WorktreePath: filepath.Join(identity, "worktree")},
	}
	for _, branch := range []string{"develop", "chats", "threads", "storages"} {
		rows = append(rows, domain.Workspace{ID: branch, WorktreePath: filepath.Join(slug, branch)})
	}
	return purgeFixture{
		home: home, outside: outside, rows: rows,
		ownRoots: map[string]string{
			filepath.Join(slug, "main", "worktree"): filepath.Join(slug, "main"),
			filepath.Join(identity, "worktree"):     identity,
		},
	}
}

func (f purgeFixture) entries(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, dir := range []string{f.home, f.outside} {
		require.NoError(t, filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				out = append(out, path)
			}
			return err
		}))
	}
	return out
}

func FuzzWorktreeRemover_NeverRemovesOutsideTheWorkspacesOwnRoot(f *testing.F) {
	for _, seed := range []string{
		"projects/P/github.com/acme/app/main/worktree",
		"projects/P/r1/workspaces/W1/worktree",
		"projects/P/github.com/acme/app/develop",
		"projects/P/github.com/acme/app/chats",
		"projects/P/github.com/acme/app/chats/worktree",
		"projects/P/github.com/acme/app/threads/worktree",
		"projects/P/github.com/acme/app/alias/worktree",
		"projects/P/github.com/acme/app/develop/../main/worktree",
		"projects/P/github.com/acme/app/worktree",
		"projects/P/worktree",
		"projects/worktree",
		"worktree",
		"",
		"$OUT/victim/worktree",
		"$OUT/victim",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, recorded string) {
		fx := newPurgeFixture(t)
		path := strings.ReplaceAll(recorded, "$OUT", fx.outside)
		if !strings.HasPrefix(recorded, "$OUT") && recorded != "" {
			path = fx.home + "/" + recorded
		}
		tomb := domain.Workspace{ID: "victim", WorktreePath: path, Status: domain.WorkspaceStatusDeleted}
		for _, row := range fx.rows {
			if row.WorktreePath == path {
				tomb.ID = row.ID
			}
		}
		before := fx.entries(t)
		rows := func(context.Context) ([]domain.Workspace, error) { return fx.rows, nil }

		_ = WorktreeRemover(fx.home, rows)(context.Background(), tomb)

		allowed, ok := fx.ownRoots[path]
		for _, entry := range before {
			if _, err := os.Lstat(entry); err == nil {
				continue
			}
			assert.True(t, ok && strings.HasPrefix(entry, allowed+"/"),
				"recorded %q removed %q, outside its own root", recorded, entry)
		}
	})
}
