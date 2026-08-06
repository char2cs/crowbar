package layout_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/layout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pathRecorder struct{ paths map[string]string }

func (r *pathRecorder) Relocate(_ context.Context, id, p string) error {
	if r.paths == nil {
		r.paths = map[string]string{}
	}
	r.paths[id] = p
	return nil
}

// The OLDEST managed layout put the checkout directly at <slug>/<branch>, with
// no "worktree" leaf — the shape every protected branch provisioned before the
// leaf existed, and the one four repos' develop/master still sit in.
//
// It has to migrate like any other managed worktree. Left behind it is a real
// directory standing exactly where this layout expects a symlink, which is the
// one thing LinkAlias and UnlinkAlias both refuse to touch — so the workspace
// opens fine and then cannot be renamed or removed.
func TestRun_BareCheckoutWithNoWorktreeLeaf(t *testing.T) {
	home := t.TempDir()
	bare := filepath.Join(home, "projects", "P", "github.com/acme/app", "develop")
	require.NoError(t, os.MkdirAll(filepath.Join(bare, "api"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bare, ".git"), []byte("gitdir: x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bare, "api", "main.go"), []byte("package main"), 0o644))

	rel := &pathRecorder{}
	res := layout.Run(context.Background(), home,
		[]layout.Workspace{{ID: "W1", ProjectID: "P", WorktreePath: bare}}, nil, rel)

	require.Equal(t, 0, res.Failed)
	assert.Equal(t, 1, res.Migrated, "a bare checkout is a managed worktree and must migrate")

	// The record names the identity-keyed worktree, and it holds the checkout.
	want := filepath.Join(home, "projects", "P", "workspaces", "W1", "worktree")
	assert.Equal(t, want, rel.paths["W1"])
	got, err := os.ReadFile(filepath.Join(want, "api", "main.go"))
	require.NoError(t, err, "the checkout must have moved intact")
	assert.Equal(t, "package main", string(got))

	// What stands at the old path is a SYMLINK — the thing rename and delete
	// require — not the real directory that made them refuse.
	info, err := os.Lstat(bare)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the old path must become an alias")
}

// An adopted checkout has no leaf either, and is the one thing that must never
// move — the guard that separates them cannot be the leaf.
func TestRun_BareAdoptedCheckoutIsNeverMoved(t *testing.T) {
	home := t.TempDir()
	// Inside the project directory, which is what makes this the dangerous case.
	repo := filepath.Join(home, "projects", "P", "github.com/acme/app", "main")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: x"), 0o644))

	rel := &pathRecorder{}
	res := layout.Run(context.Background(), home,
		[]layout.Workspace{{ID: "W1", ProjectID: "P", WorktreePath: repo, AdoptedPath: repo}}, nil, rel)

	assert.Equal(t, 1, res.Skipped)
	assert.Empty(t, rel.paths, "an adopted checkout's record must not be rewritten")
	info, err := os.Lstat(repo)
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "an adopted checkout must stay a real directory")
	assert.Zero(t, info.Mode()&os.ModeSymlink)
}

// A bare path with no .git is a branch-prefix DIRECTORY, not a workspace:
// <slug>/feature stands over <slug>/feature/x/worktree. Moving it would take
// every branch beneath it along, which is the shape of the run that once
// relocated a whole tree of repositories.
func TestRun_BareContainerDirectoryIsNeverMoved(t *testing.T) {
	home := t.TempDir()
	container := filepath.Join(home, "projects", "P", "github.com/acme/app", "feature")
	child := filepath.Join(container, "x", "worktree")
	require.NoError(t, os.MkdirAll(child, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(child, ".git"), []byte("gitdir: x"), 0o644))

	rel := &pathRecorder{}
	res := layout.Run(context.Background(), home,
		[]layout.Workspace{{ID: "W1", ProjectID: "P", WorktreePath: container}}, nil, rel)

	assert.Equal(t, 1, res.Skipped)
	assert.Empty(t, rel.paths)
	info, err := os.Lstat(container)
	require.NoError(t, err)
	assert.True(t, info.IsDir() && info.Mode()&os.ModeSymlink == 0,
		"a container directory must stay exactly where it is")
	assert.DirExists(t, child, "the branches beneath it must be untouched")
}

// A pre-leaf workspace's chats do NOT live inside it. ChatsDir is
// filepath.Dir(worktreePath)/chats, which for <slug>/<branch> resolves to
// <slug>/chats — a directory SHARED by every pre-leaf workspace of that repo,
// and one the worktree move does not touch.
//
// Left behind, the conversation ledgers are orphaned: the new root derives an
// empty chats dir and the history reads as gone. Each workspace's own chats have
// to come with it, and only its own.
func TestRun_BareCheckoutCarriesItsChatLedgers(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "P", "github.com/acme/app")
	sharedChats := filepath.Join(slug, "chats")

	mk := func(branch, chatID, turn string) string {
		wt := filepath.Join(slug, branch)
		require.NoError(t, os.MkdirAll(wt, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: x"), 0o644))
		ledger := filepath.Join(sharedChats, chatID, "ledger")
		require.NoError(t, os.MkdirAll(ledger, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(ledger, "0001.turn"), []byte(turn), 0o644))
		return wt
	}
	devWT := mk("develop", "chat-dev", "develop history")
	mainWT := mk("master", "chat-main", "master history")
	// Shared runner scratch belonging to neither.
	require.NoError(t, os.MkdirAll(filepath.Join(sharedChats, "runners"), 0o755))

	rel := &pathRecorder{}
	res := layout.Run(context.Background(), home, []layout.Workspace{
		{ID: "W1", ProjectID: "P", WorktreePath: devWT, ChatIDs: []string{"chat-dev"}},
		{ID: "W2", ProjectID: "P", WorktreePath: mainWT, ChatIDs: []string{"chat-main"}},
	}, nil, rel)
	require.Equal(t, 0, res.Failed)
	require.Equal(t, 2, res.Migrated)

	// Each workspace's ledger now sits in ITS OWN root, beside its worktree.
	for _, tc := range []struct{ id, chat, want string }{
		{"W1", "chat-dev", "develop history"},
		{"W2", "chat-main", "master history"},
	} {
		root := filepath.Dir(rel.paths[tc.id])
		got, err := os.ReadFile(filepath.Join(root, "chats", tc.chat, "ledger", "0001.turn"))
		require.NoError(t, err, "%s: its chat ledger must have come along", tc.id)
		assert.Equal(t, tc.want, string(got))
	}
	// And no workspace took the other's.
	assert.NoDirExists(t, filepath.Join(filepath.Dir(rel.paths["W1"]), "chats", "chat-main"))
	assert.NoDirExists(t, filepath.Join(filepath.Dir(rel.paths["W2"]), "chats", "chat-dev"))
}
