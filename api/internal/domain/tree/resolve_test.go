package tree_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestResolveWorkspace_WalksToTheNearestWorkspaceThenRepoThenHome(t *testing.T) {
	tr, f := pinTree()
	assert.Equal(t, "ws-feat", tree.ResolveWorkspace(tr, "ws-feat", f))
	assert.Equal(t, "ws-feat", tree.ResolveWorkspace(tr, "c-follow", f), "a chat resolves to its own workspace")
	assert.Equal(t, "home-alpha", tree.ResolveWorkspace(tr, "f-alpha", f), "a repo folder to the repo's home checkout")
	assert.Equal(t, "home-alpha", tree.ResolveWorkspace(tr, "r-alpha", f))
	assert.Equal(t, "home-p1", tree.ResolveWorkspace(tr, "f-ideas", f))
	assert.Equal(t, "home-p1", tree.ResolveWorkspace(tr, "", f))
	moved, err := tr.Move("c-follow", "f-alpha", 0, f)
	require.NoError(t, err)
	assert.Equal(t, "ws-feat", tree.ResolveWorkspace(moved, "c-follow", f), "a chat filed away keeps its own")
}

func TestAncestors_StepsThroughFoldersAndCountsAWorkspaceAsItsOwningChat(t *testing.T) {
	tr, f := pinTree()
	f.BoundTo["c-deep"] = "ws-fork"
	tr, err := tr.Insert(tree.Node{ID: "f-in", Kind: tree.KindFolder, Name: "in"}, "ws-fork", 0, f)
	require.NoError(t, err)
	tr, err = tr.Insert(tree.Node{ID: "c-mid", Kind: tree.KindChat}, "f-in", 0, f)
	require.NoError(t, err)
	tr, err = tr.Insert(tree.Node{ID: "c-deep", Kind: tree.KindChat}, "c-mid", 0, f)
	require.NoError(t, err)
	owners := map[string]string{"ws-fork": "oc-fork", "ws-feat": "oc-feat"}
	ownerOf := func(ws string) string { return owners[ws] }
	assert.Equal(t, []string{"c-mid", "oc-fork", "oc-feat"}, tree.Ancestors(tr, "c-deep", ownerOf))
	assert.Equal(t, []string{"oc-feat"}, tree.Ancestors(tr, "ws-fork", ownerOf))
	delete(owners, "ws-feat")
	assert.Equal(t, []string{"c-mid", "oc-fork"}, tree.Ancestors(tr, "c-deep", ownerOf), "an unowned workspace adds nothing")
}

func TestAncestors_OfARootChatIsEmpty(t *testing.T) {
	tr, _ := pinTree()
	none := func(string) string { return "" }
	assert.Empty(t, tree.Ancestors(tr, "c-notes", none))
	assert.Empty(t, tree.Ancestors(tr, "c-alpha-q", none), "a repo ends the walk")
	assert.Empty(t, tree.Ancestors(tr, "c-draft", none))
	assert.Empty(t, tree.Ancestors(tr, "nowhere", none))
}
