package tree_test

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// level is parentID's children as "id" in order, asserting each order is its index.
func level(t *testing.T, tr tree.Tree, parentID string) []string {
	t.Helper()
	var ids []string
	for i, n := range tr.Children(parentID) {
		assert.Equal(t, i, n.Order, "%s's order under %q", n.ID, parentID)
		ids = append(ids, n.ID)
	}
	return ids
}

func TestMove_RenumbersBothLevelsDensely(t *testing.T) {
	tr, f := pinTree()
	out, err := tr.Move("ws-release", "f-alpha", 0, f)
	require.NoError(t, err)
	assert.Equal(t, []string{"c-alpha-q", "ws-feat", "f-alpha", "ws-b"}, level(t, out, "r-alpha"))
	assert.Equal(t, []string{"ws-release", "c-try"}, level(t, out, "f-alpha"))
	assert.Equal(t, tr.Nodes["c-follow"], out.Nodes["c-follow"], "nobody else is touched")
	assert.Equal(t, []string{"c-notes", "r-alpha", "f-ideas"}, level(t, out, ""))
}

func TestMove_WithinOneLevelReorders(t *testing.T) {
	tr, f := pinTree()
	out, err := tr.Move("f-ideas", "", 0, f)
	require.NoError(t, err)
	assert.Equal(t, []string{"f-ideas", "c-notes", "r-alpha"}, level(t, out, ""))
	out, err = out.Move("f-ideas", "", 99, f)
	require.NoError(t, err)
	assert.Equal(t, []string{"c-notes", "r-alpha", "f-ideas"}, level(t, out, ""), "an index past the end appends")
	out, err = out.Move("f-ideas", "", -3, f)
	require.NoError(t, err)
	assert.Equal(t, []string{"f-ideas", "c-notes", "r-alpha"}, level(t, out, ""), "a negative index goes first")
}

func TestInsert_AtAnIndexShiftsLaterSiblings(t *testing.T) {
	tr, f := pinTree()
	f.BoundTo["c-new"] = "home-alpha"
	out, err := tr.Insert(tree.Node{ID: "c-new", Kind: tree.KindChat}, "r-alpha", 1, f)
	require.NoError(t, err)
	assert.Equal(t, []string{"c-alpha-q", "c-new", "ws-release", "ws-feat", "f-alpha", "ws-b"}, level(t, out, "r-alpha"))
	_, err = tr.Insert(tree.Node{ID: "c-new", Kind: tree.KindChat}, "", 0, f)
	requireRefused(t, tree.ErrCrossContext, err, "a repo chat at the project root")
	_, err = tr.Insert(tree.Node{ID: "c-new", Kind: tree.KindChat}, "ws-b", 0, f)
	requireRefused(t, tree.ErrCrossWorkspace, err)
	_, err = tr.Insert(tree.Node{ID: "f-x", Kind: tree.KindFolder}, "", 0, f)
	requireRefused(t, tree.ErrNameRequired, err)
	_, err = tr.Insert(tree.Node{ID: "f-x", Kind: tree.KindFolder, Name: "x"}, "nowhere", 0, f)
	requireRefused(t, tree.ErrUnknownNode, err)
}

func TestInsert_RefusesAnExistingID(t *testing.T) {
	tr, f := pinTree()
	_, err := tr.Insert(tree.Node{ID: "c-try", Kind: tree.KindChat}, "f-alpha", 0, f)
	requireRefused(t, tree.ErrExists, err)
}

func TestRemove_PromoteMovesChildrenToTheFoldersPlace(t *testing.T) {
	tr, f := pinTree()
	tr, err := tr.Move("ws-release", "f-alpha", 1, f)
	require.NoError(t, err)
	out, gone, err := tr.Remove("f-alpha", true)
	require.NoError(t, err)
	assert.Equal(t, []string{"f-alpha"}, gone)
	assert.Equal(t, []string{"c-alpha-q", "ws-feat", "c-try", "ws-release", "ws-b"}, level(t, out, "r-alpha"))
}

func TestRemove_CascadeReturnsTheWholeSubtree(t *testing.T) {
	tr, _ := pinTree()
	out, gone, err := tr.Remove("ws-feat", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"ws-feat", "c-follow", "ws-fork"}, gone)
	assert.Equal(t, []string{"c-alpha-q", "ws-release", "f-alpha", "ws-b"}, level(t, out, "r-alpha"))
	assert.NotContains(t, out.Nodes, "c-follow")
	_, _, err = tr.Remove("nowhere", false)
	requireRefused(t, tree.ErrUnknownNode, err)
}

func TestRename_RequiresANameAndAFolder(t *testing.T) {
	tr, _ := pinTree()
	_, err := tr.Rename("f-alpha", "  ")
	requireRefused(t, tree.ErrNameRequired, err)
	_, err = tr.Rename("c-try", "x")
	requireRefused(t, tree.ErrNotAContainer, err)
	out, err := tr.Rename("f-alpha", " Spikes ")
	require.NoError(t, err)
	assert.Equal(t, "Spikes", out.Nodes["f-alpha"].Name)
}

func TestPromote_ReplacesAChatNodeWithAWorkspaceNodeInPlace(t *testing.T) {
	tr, f := pinTree()
	f.BoundTo["c-thread"] = "ws-feat"
	tr, err := tr.Insert(tree.Node{ID: "c-thread", Kind: tree.KindChat}, "c-follow", 0, f)
	require.NoError(t, err)
	out, err := tr.Promote("c-follow", "ws-follow")
	require.NoError(t, err)
	assert.Equal(t, tree.Node{ID: "ws-follow", Kind: tree.KindWorkspace, ParentID: "ws-feat", Order: 0}, out.Nodes["ws-follow"])
	assert.NotContains(t, out.Nodes, "c-follow")
	assert.Equal(t, "ws-follow", out.Nodes["c-thread"].ParentID)
	_, err = tr.Promote("ws-feat", "ws-x")
	requireRefused(t, tree.ErrNotAContainer, err)
	_, err = tr.Promote("c-follow", "ws-b")
	requireRefused(t, tree.ErrExists, err)
}

func TestVerbs_NeverMutateTheReceiver(t *testing.T) {
	tr, f := pinTree()
	before := maps.Clone(tr.Nodes)
	_, _ = tr.Move("ws-release", "f-alpha", 0, f)
	_, _ = tr.Move("f-alpha", "f-ideas", 0, f) // refused after its change was built
	_, _ = tr.Insert(tree.Node{ID: "f-x", Kind: tree.KindFolder, Name: "x"}, "", 0, f)
	_, _ = tr.Rename("f-alpha", "y")
	_, _, _ = tr.Remove("f-alpha", true)
	_, _, _ = tr.Remove("ws-feat", false)
	_, _ = tr.Promote("c-follow", "ws-follow")
	assert.Equal(t, before, tr.Nodes)
}
