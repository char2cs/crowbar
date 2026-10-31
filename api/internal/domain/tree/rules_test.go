package tree_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// pinTree is T1's pinned hierarchy with explicit containers, plus a second
// branch (ws-b) to file things across.
func pinTree() (tree.Tree, tree.Facts) {
	t := tree.Empty("p1")
	for _, n := range []tree.Node{
		{ID: "c-notes", Kind: tree.KindChat, Order: 0},
		{ID: "r-alpha", Kind: tree.KindRepo, Order: 1},
		{ID: "c-alpha-q", Kind: tree.KindChat, ParentID: "r-alpha", Order: 0},
		{ID: "ws-release", Kind: tree.KindWorkspace, ParentID: "r-alpha", Order: 1},
		{ID: "ws-feat", Kind: tree.KindWorkspace, ParentID: "r-alpha", Order: 2},
		{ID: "c-follow", Kind: tree.KindChat, ParentID: "ws-feat", Order: 0},
		{ID: "ws-fork", Kind: tree.KindWorkspace, ParentID: "ws-feat", Order: 1},
		{ID: "f-alpha", Kind: tree.KindFolder, ParentID: "r-alpha", Order: 3, Name: "Experiments"},
		{ID: "c-try", Kind: tree.KindChat, ParentID: "f-alpha", Order: 0},
		{ID: "ws-b", Kind: tree.KindWorkspace, ParentID: "r-alpha", Order: 4},
		{ID: "c-b", Kind: tree.KindChat, ParentID: "ws-b", Order: 0},
		{ID: "f-ideas", Kind: tree.KindFolder, Order: 2, Name: "Ideas"},
		{ID: "c-draft", Kind: tree.KindChat, ParentID: "f-ideas", Order: 0},
		{ID: "r-beta", Kind: tree.KindRepo, ParentID: "f-ideas", Order: 1},
	} {
		t.Nodes[n.ID] = n
	}
	f := tree.Facts{
		ProjectHome: "home-p1",
		RepoHome:    map[string]string{"r-alpha": "home-alpha", "r-beta": "home-beta"},
		Repo: map[string]string{
			"home-p1": "", "home-alpha": "r-alpha", "ws-release": "r-alpha", "ws-feat": "r-alpha",
			"ws-fork": "r-alpha", "ws-b": "r-alpha", "home-beta": "r-beta",
		},
		BoundTo: map[string]string{
			"c-notes": "home-p1", "c-alpha-q": "home-alpha", "c-follow": "ws-feat", "c-try": "home-alpha",
			"c-b": "ws-b", "c-draft": "home-p1",
		},
		ForkParent: map[string]string{"ws-fork": "ws-feat"},
		Working:    map[string]bool{},
	}
	return t, f
}

func requireRefused(t *testing.T, want *problem.Error, err error, msgAndArgs ...any) {
	t.Helper()
	require.ErrorIs(t, err, want, msgAndArgs...)
	assert.Equal(t, want.Code, problem.CodeOf(err), msgAndArgs...)
}

func TestMove_RefusesACycle(t *testing.T) {
	tr, f := pinTree()
	_, err := tr.Move("f-alpha", "f-alpha", 0, f)
	requireRefused(t, tree.ErrCycle, err)
	_, err = tr.Move("ws-feat", "ws-fork", 0, f)
	requireRefused(t, tree.ErrCycle, err)
}

func TestMove_RefusesAFolderUnderAChat(t *testing.T) {
	tr, f := pinTree()
	_, err := tr.Move("f-alpha", "c-alpha-q", 0, f)
	requireRefused(t, tree.ErrNotAContainer, err)
}

func TestMove_RefusesARepoBelowProjectLevel(t *testing.T) {
	tr, f := pinTree()
	_, err := tr.Move("r-beta", "ws-feat", 0, f)
	requireRefused(t, tree.ErrNotAContainer, err)
	_, err = tr.Move("r-beta", "f-alpha", 0, f)
	requireRefused(t, tree.ErrNotAContainer, err, "a repo folder is below project level")
	out, err := tr.Move("r-alpha", "f-ideas", 0, f)
	require.NoError(t, err)
	assert.Equal(t, "f-ideas", out.Nodes["r-alpha"].ParentID)
}

func TestMove_KeepsANodeInItsContext(t *testing.T) {
	tr, f := pinTree()
	_, err := tr.Move("f-alpha", "f-ideas", 0, f)
	requireRefused(t, tree.ErrCrossContext, err)
	_, err = tr.Move("c-notes", "f-alpha", 0, f)
	requireRefused(t, tree.ErrCrossContext, err)
	_, err = tr.Move("c-notes", "r-alpha", 0, f)
	requireRefused(t, tree.ErrCrossContext, err)
}

func TestMove_RefusesAChatLeavingItsWorkspace(t *testing.T) {
	tr, f := pinTree()
	_, err := tr.Move("c-follow", "ws-b", 0, f)
	requireRefused(t, tree.ErrCrossWorkspace, err)
	_, err = tr.Move("c-follow", "c-b", 0, f)
	requireRefused(t, tree.ErrCrossWorkspace, err)
	_, err = tr.Move("c-follow", "f-alpha", 0, f)
	require.NoError(t, err, "a folder holds a chat of any workspace")
	moved, err := tr.Move("f-alpha", "ws-b", 0, f)
	require.NoError(t, err, "a folder under another branch keeps its chats")
	_, err = moved.Move("c-follow", "f-alpha", 0, f)
	require.NoError(t, err)
}

func TestMove_KeepsAForkUnderItsForkParent(t *testing.T) {
	tr, f := pinTree()
	_, err := tr.Move("ws-fork", "ws-b", 0, f)
	requireRefused(t, tree.ErrForkChain, err)
	_, err = tr.Move("ws-fork", "r-alpha", 0, f)
	requireRefused(t, tree.ErrForkChain, err)
	inFeat, err := tr.Insert(tree.Node{ID: "f-feat", Kind: tree.KindFolder, Name: "x"}, "ws-feat", 0, f)
	require.NoError(t, err)
	_, err = inFeat.Move("ws-fork", "f-feat", 0, f)
	require.NoError(t, err, "a folder inside the fork parent")
	_, err = inFeat.Move("ws-b", "f-feat", 0, f)
	requireRefused(t, tree.ErrForkChain, err, "a branch filed in a folder under another branch")
	_, err = tr.Move("f-alpha", "ws-feat", 0, f)
	require.NoError(t, err)
	withRelease, err := tr.Move("ws-release", "f-alpha", 0, f)
	require.NoError(t, err)
	_, err = withRelease.Move("f-alpha", "ws-b", 0, f)
	requireRefused(t, tree.ErrForkChain, err, "a folder carrying a branch under another branch")
}

func TestMove_RefusesASubtreeHoldingAWorkingChat(t *testing.T) {
	tr, f := pinTree()
	f.Working["c-try"] = true
	_, err := tr.Move("f-alpha", "ws-feat", 0, f)
	requireRefused(t, tree.ErrSubtreeWorking, err)
	assert.Equal(t, "Stop the running chat before moving it", tree.ErrSubtreeWorking.Message)
	_, err = tr.Move("c-alpha-q", "f-alpha", 0, f)
	require.NoError(t, err, "a working chat elsewhere does not refuse the move")
}

func TestMove_TreatsFoldersAsTransparentForResolution(t *testing.T) {
	tr, f := pinTree()
	nested, err := tr.Insert(tree.Node{ID: "f-in", Kind: tree.KindFolder, Name: "in"}, "f-alpha", 0, f)
	require.NoError(t, err)
	_, err = nested.Move("ws-release", "f-in", 0, f)
	require.NoError(t, err, "two folders deep under the repo still resolve to its home checkout")
	_, err = nested.Move("ws-fork", "f-in", 0, f)
	requireRefused(t, tree.ErrForkChain, err)
}
