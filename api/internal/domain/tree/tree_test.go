package tree_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestKindOf_ParsesEveryStoredKindAndRefusesTheRest(t *testing.T) {
	for _, k := range []string{"folder", "repo", "workspace", "chat"} {
		got, err := tree.KindOf(k)
		require.NoError(t, err)
		assert.Equal(t, tree.Kind(k), got)
	}
	_, err := tree.KindOf("branch")
	assert.Equal(t, problem.InvalidArgument, problem.CodeOf(err))
}

func TestFolder_InHomeIsAHomeFolderOfThatHome(t *testing.T) {
	assert.True(t, tree.Folder{HomeID: "h1"}.InHome("h1"))
	assert.False(t, tree.Folder{HomeID: "h1"}.InHome("h2"))
	assert.False(t, tree.Folder{RepoID: "r1", HomeID: "h1"}.InHome("h1"), "a repo folder is in no home")
}

func TestTree_MergeAddsWhatIsMissingAndKeepsWhatIsThere(t *testing.T) {
	a := tree.Empty("p1")
	a.Nodes["n1"] = tree.Node{ID: "n1", Kind: tree.KindChat, Order: 1}
	b := tree.Empty("p1")
	b.Nodes["n1"] = tree.Node{ID: "n1", Kind: tree.KindChat, Order: 9}
	b.Chats["c1"] = tree.Placement{ParentID: "n1"}

	got := a.Merge(b)

	assert.Equal(t, 1, got.Nodes["n1"].Order)
	assert.Equal(t, tree.Placement{ParentID: "n1"}, got.Chats["c1"])
	assert.NotContains(t, a.Chats, "c1", "the receiver is not changed")
}

func TestTree_WithoutTakesEveryRecordOfAnID(t *testing.T) {
	a := tree.Empty("p1")
	a.Nodes["f1"] = tree.Node{ID: "f1", Kind: tree.KindFolder}
	a.Folders["f1"] = tree.Folder{ID: "f1", Name: "F"}
	a.Chats["c1"] = tree.Placement{ParentID: "f1"}

	rest, taken := a.Without("f1")

	assert.False(t, rest.Holds("f1"))
	assert.True(t, rest.Holds("c1"))
	assert.Equal(t, "F", taken.Folders["f1"].Name)
	assert.True(t, a.Holds("f1"), "the receiver is not changed")
}

func TestOrUnplaced_ARowNotWrittenYetIsTheRootsFirstSlot(t *testing.T) {
	n, err := tree.OrUnplaced(tree.Node{}, fmt.Errorf("tree: node r1: %w", tree.ErrUnknownNode))

	require.NoError(t, err)
	assert.Equal(t, tree.Node{}, n)
}

func TestOrUnplaced_AnyOtherFailureStays(t *testing.T) {
	broken := errors.New("disk gone")

	_, err := tree.OrUnplaced(tree.Node{}, broken)

	require.ErrorIs(t, err, broken)
}

func TestOrUnplaced_ARowReadsAsItself(t *testing.T) {
	want := tree.Node{ID: "r1", ParentID: "f1", Order: 2}

	n, err := tree.OrUnplaced(want, nil)

	require.NoError(t, err)
	assert.Equal(t, want, n)
}
