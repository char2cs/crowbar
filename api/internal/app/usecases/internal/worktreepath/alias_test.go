package worktreepath

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkAlias_PublishesASymlinkToTheRoot(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "workspaces", "w1")
	alias := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo", "main")
	require.NoError(t, os.MkdirAll(root, 0o755))

	require.NoError(t, LinkAlias(alias, root))

	target, err := os.Readlink(alias)
	require.NoError(t, err)
	assert.Equal(t, root, target)
}

// Renaming testing -> testing/x needs <slug>/testing to become a directory while
// it is still the alias for `testing`. MkdirAll would walk THROUGH that symlink
// and plant the new alias inside the workspace root it points at.
func TestRegression_LinkAlias_ReplacesAnAncestorAliasStandingInTheWay(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo")
	root := filepath.Join(home, "projects", "p1", "workspaces", "w1")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, LinkAlias(filepath.Join(slug, "testing"), root))

	nested := filepath.Join(slug, "testing", "really-big")
	require.NoError(t, LinkAlias(nested, root))

	target, err := os.Readlink(nested)
	require.NoError(t, err)
	assert.Equal(t, root, target)
	// The nested alias must live in the ALIAS tree, never inside the root it
	// points at.
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "the workspace root must not have grown an alias inside it")
}

// A real directory at the alias path is an unmigrated workspace's actual tree.
// Replacing or unlinking it would destroy a worktree.
func TestLinkAlias_RefusesToReplaceARealDirectory(t *testing.T) {
	home := t.TempDir()
	alias := filepath.Join(home, "projects", "p1", "slug", "main")
	require.NoError(t, os.MkdirAll(filepath.Join(alias, "worktree"), 0o755))

	err := LinkAlias(alias, filepath.Join(home, "projects", "p1", "workspaces", "w1"))

	require.Error(t, err)
	assert.DirExists(t, filepath.Join(alias, "worktree"), "the real tree must survive")
}

func TestUnlinkAlias_RemovesTheLinkAndTheParentsItEmpties(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo")
	root := filepath.Join(home, "projects", "p1", "workspaces", "w1")
	require.NoError(t, os.MkdirAll(root, 0o755))
	alias := filepath.Join(slug, "feature", "x")
	require.NoError(t, LinkAlias(alias, root))

	require.NoError(t, UnlinkAlias(alias, slug))

	assert.NoDirExists(t, filepath.Join(slug, "feature"),
		"the emptied nested parent must not squat the name")
	assert.DirExists(t, slug, "the walk stops at the slug directory")
}

func TestUnlinkAlias_RefusesARealDirectory(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "p1", "slug")
	alias := filepath.Join(slug, "main")
	require.NoError(t, os.MkdirAll(filepath.Join(alias, "worktree"), 0o755))

	err := UnlinkAlias(alias, slug)

	require.Error(t, err)
	assert.DirExists(t, filepath.Join(alias, "worktree"))
}

func TestWorkspaceRootByID_IsKeyedByIdentityAlone(t *testing.T) {
	root, err := WorkspaceRootByID("/crow", "p1", "w1")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/p1/workspaces/w1", root)
	assert.Equal(t, "/crow/projects/p1/workspaces/w1/worktree", WorktreeLeaf(root))
}

func TestWorkspaceRootByID_RejectsAnIDThatEscapesTheProject(t *testing.T) {
	_, err := WorkspaceRootByID("/crow", "p1", "../../../tmp/pwned")
	require.Error(t, err)
}

// ResolveAlias answers only for a real link: a missing path and a real directory
// are both "not an alias", which is what stops a caller treating an unmigrated
// workspace's own tree as one.
func TestResolveAlias_OnlyAnswersForALink(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "workspaces", "w1")
	alias := filepath.Join(home, "projects", "p1", "slug", "main")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, LinkAlias(alias, root))

	target, ok := ResolveAlias(alias)
	require.True(t, ok)
	assert.Equal(t, root, target)

	_, ok = ResolveAlias(filepath.Join(home, "nothing-here"))
	assert.False(t, ok, "a missing path is not an alias")

	real := filepath.Join(home, "projects", "p1", "slug", "real")
	require.NoError(t, os.MkdirAll(real, 0o755))
	_, ok = ResolveAlias(real)
	assert.False(t, ok, "a real directory is not an alias")
}

func TestLinkAlias_RejectsEmptyArguments(t *testing.T) {
	require.Error(t, LinkAlias("", "/root"))
	require.Error(t, LinkAlias("/alias", ""))
}

// Unlinking something that was never there is a no-op, so a re-driven teardown
// is not an error.
func TestUnlinkAlias_MissingIsANoOp(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, UnlinkAlias(filepath.Join(home, "gone"), home))
	require.NoError(t, UnlinkAlias("", home))
}

func TestAliasDir_RejectsAnEmptyComponent(t *testing.T) {
	_, err := AliasDir("/crow", "p1", "", "main")
	require.Error(t, err)
}

func TestWorkspaceRootByID_RejectsEmptyComponents(t *testing.T) {
	for _, args := range [][3]string{{"", "p1", "w1"}, {"/crow", "", "w1"}, {"/crow", "p1", ""}} {
		_, err := WorkspaceRootByID(args[0], args[1], args[2])
		require.Error(t, err, "%v", args)
	}
}
