package tree_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/tree"
)

func orders(
	t *testing.T,
	plan tree.Tree,
	rows ...string,
) []int {
	t.Helper()
	out := make([]int, 0, len(rows))
	for _, id := range rows {
		node, ok := plan.Node(id)
		require.True(t, ok, "plan must hold %s", id)
		out = append(out, node.Order)
	}
	return out
}

func level(
	container string,
	rows ...string,
) []tree.Node {
	nodes := make([]tree.Node, 0, len(rows))
	for i, id := range rows {
		nodes = append(nodes, tree.Node{ID: id, ParentID: container, Order: i})
	}
	return nodes
}

func TestNew_CopiesTheRowsItIsGiven(t *testing.T) {
	rows := level("", "a", "b")
	plan := tree.New(rows)

	plan.Reorder("", "b", 0)

	assert.Equal(t, 0, rows[0].Order, "the caller's own slice is untouched")
	assert.Equal(t, 1, rows[1].Order)
	assert.Equal(t, []int{1, 0}, orders(t, plan, "a", "b"))
}

func TestNode_AnswersWhetherTheForestHoldsTheRow(t *testing.T) {
	plan := tree.New(level("", "a"))

	node, ok := plan.Node("a")
	require.True(t, ok)
	assert.Equal(t, "a", node.ID)

	_, ok = plan.Node("missing")
	assert.False(t, ok)
}

func TestMembers_ReturnsOnlyTheContainersOwnRows(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "root-a"},
		{ID: "box"},
		{ID: "in-box", ParentID: "box"},
	})

	assert.Equal(t, []string{"in-box"}, ids(plan.Members("box")))
	assert.ElementsMatch(t, []string{"root-a", "box"}, ids(plan.Members("")))
	assert.Empty(t, plan.Members("nowhere"))
}

func TestNextSlot_IsTheEndOfTheSiblingSpace(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "a"},
		{ID: "b"},
		{ID: "deep", ParentID: "a"},
	})

	assert.Equal(t, 2, plan.NextSlot(""))
	assert.Equal(t, 1, plan.NextSlot("a"))
	assert.Equal(t, 0, plan.NextSlot("nowhere"))
}

func TestIndexOf_ReportsThePositionAndZeroForARowTheLevelLacks(t *testing.T) {
	plan := tree.New(level("", "a", "b", "c"))

	assert.Equal(t, 2, plan.IndexOf("", "c"))
	assert.Equal(t, 0, plan.IndexOf("", "elsewhere"),
		"a row the level does not hold reads as the front, never as -1")
}

func TestAdd_JoinsTheForestAlreadyChanged(t *testing.T) {
	plan := tree.New(level("", "a"))

	plan.Add(tree.Node{ID: "new", Order: 1})

	node, ok := plan.Node("new")
	require.True(t, ok)
	assert.Equal(t, 1, node.Order)
	assert.Equal(t, []string{"new"}, plan.Dirty(),
		"a row the plan invented has by definition never been written")
	assert.Equal(t, []string{"a", "new"}, ids(plan.Members("")))
}

func TestDrop_RemovesTheRowAndItsPendingWrite(t *testing.T) {
	plan := tree.New(level("", "a", "b", "c"))
	plan.Reorder("", "c", 0)
	require.Equal(t, []string{"a", "b", "c"}, plan.Dirty())

	plan.Drop("c")

	_, ok := plan.Node("c")
	assert.False(t, ok)
	assert.Equal(t, []string{"a", "b"}, plan.Dirty(),
		"a row that is going away must not also be reported as a row to write")
	assert.Equal(t, []int{1, 2}, orders(t, plan, "a", "b"),
		"the index map survives the hole the delete leaves")
}

func TestDrop_OfAnUnknownRowIsANoOp(t *testing.T) {
	plan := tree.New(level("", "a"))

	plan.Drop("missing")

	assert.Empty(t, plan.Dirty())
	assert.Equal(t, []string{"a"}, ids(plan.Members("")))
}

func TestSetParent_MovesTheRowAndRecordsTheChange(t *testing.T) {
	plan := tree.New([]tree.Node{{ID: "box"}, {ID: "a"}})

	plan.SetParent("a", "box")

	assert.Equal(t, []string{"a"}, ids(plan.Members("box")))
	assert.Equal(t, []string{"a"}, plan.Dirty())
}

func TestSetParent_IgnoresAnUnchangedOrUnknownRow(t *testing.T) {
	plan := tree.New([]tree.Node{{ID: "a", ParentID: "box"}})

	plan.SetParent("a", "box")
	plan.SetParent("missing", "box")

	assert.Empty(t, plan.Dirty(), "a move that moves nothing is not a write")
}

func TestTouch_RecordsAChangeTheTreeDoesNotModel(t *testing.T) {
	plan := tree.New(level("", "a"))

	plan.Touch("a")
	plan.Touch("missing")

	assert.Equal(t, []string{"a"}, plan.Dirty())
}

func TestReorder_DensifiesTheWholeLevel(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "a", Order: 4},
		{ID: "b", Order: 9},
		{ID: "c", Order: 11},
	})

	plan.Reorder("", "", -1)

	assert.Equal(t, []int{0, 1, 2}, orders(t, plan, "a", "b", "c"))
	assert.Equal(t, []string{"a", "b", "c"}, plan.Dirty())
}

func TestReorder_LiftsThePlacedRowToItsTarget(t *testing.T) {
	plan := tree.New(level("", "a", "b", "c"))

	plan.Reorder("", "c", 0)

	assert.Equal(t, []int{1, 2, 0}, orders(t, plan, "a", "b", "c"))
	assert.Equal(t, []string{"c", "a", "b"}, ids(plan.Members("")))
}

func TestReorder_ClampsAnOutOfRangeTarget(t *testing.T) {
	plan := tree.New(level("", "a", "b", "c"))

	plan.Reorder("", "a", 99)

	assert.Equal(t, []int{2, 0, 1}, orders(t, plan, "a", "b", "c"))
}

func TestReorder_WithNoPlacedRowOnlyDensifies(t *testing.T) {
	plan := tree.New([]tree.Node{{ID: "a", Order: 3}, {ID: "b", Order: 7}})

	plan.Reorder("", "", 0)

	assert.Equal(t, []int{0, 1}, orders(t, plan, "a", "b"))
}

func TestReorder_WithANegativeTargetLeavesThePlacedRowWhereItIs(t *testing.T) {
	plan := tree.New(level("", "a", "b", "c"))

	plan.Reorder("", "c", -1)

	assert.Equal(t, []int{0, 1, 2}, orders(t, plan, "a", "b", "c"))
}

func TestReorder_IgnoresAPlacedRowTheLevelDoesNotHold(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "a", Order: 5},
		{ID: "b", Order: 6},
		{ID: "gone", ParentID: "box"},
	})

	plan.Reorder("", "gone", 0)

	assert.Equal(t, []int{0, 1}, orders(t, plan, "a", "b"))
	assert.Equal(t, []string{"a", "b"}, plan.Dirty(),
		"the level densifies without the absent row being appended to it")
}

func TestReorder_RecordsNothingWhenTheLevelIsAlreadyDense(t *testing.T) {
	plan := tree.New(level("", "a", "b"))

	plan.Reorder("", "", -1)

	assert.Empty(t, plan.Dirty())
}

func TestReparent_HandsTheChildrenOverWithoutTouchingTheContainer(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "box"},
		{ID: "a", ParentID: "box"},
		{ID: "b", ParentID: "box"},
		{ID: "deep", ParentID: "a"},
	})

	plan.Reparent("box", "")

	assert.ElementsMatch(t, []string{"box", "a", "b"}, ids(plan.Members("")))
	assert.Equal(t, []string{"deep"}, ids(plan.Members("a")),
		"a grandchild follows its own parent, it is not flattened")
	node, ok := plan.Node("box")
	require.True(t, ok, "reparenting empties a container, it does not remove it")
	assert.Equal(t, "", node.ParentID)
	assert.Equal(t, []string{"a", "b"}, plan.Dirty())
}

func TestReparent_LeavesRowsTheCallerAlreadyMovedAlone(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "box", ParentID: "host"},
		{ID: "host"},
		{ID: "narrow", ParentID: "box"},
		{ID: "wide", ParentID: "box"},
	})

	plan.SetParent("narrow", "")
	plan.Reparent("box", "host")

	assert.ElementsMatch(t, []string{"host", "narrow"}, ids(plan.Members("")))
	assert.ElementsMatch(t, []string{"box", "wide"}, ids(plan.Members("host")))
}

func TestReaches_WalksUpAcrossEveryEdge(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "outer"},
		{ID: "host", ParentID: "outer"},
		{ID: "inner", ParentID: "host"},
		{ID: "aside"},
	})

	assert.True(t, plan.Reaches("inner", "outer"))
	assert.True(t, plan.Reaches("outer", "outer"), "a container reaches itself")
	assert.False(t, plan.Reaches("aside", "outer"))
	assert.False(t, plan.Reaches("", "outer"), "the root is above everything")
}

func TestReaches_WalksPastADanglingEdge(t *testing.T) {
	plan := tree.New([]tree.Node{{ID: "orphan", ParentID: "gone"}})

	assert.False(t, plan.Reaches("orphan", "subject"))
}

func TestReaches_TerminatesOnAPersistedCycle(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "a", ParentID: "b"},
		{ID: "b", ParentID: "a"},
	})

	assert.False(t, plan.Reaches("a", "subject"))
	assert.True(t, plan.Reaches("a", "b"))
}

func TestDirty_IsSortedSoAPartialFailureIsReproducible(t *testing.T) {
	plan := tree.New([]tree.Node{{ID: "c"}, {ID: "b"}, {ID: "a"}})

	plan.Touch("c")
	plan.Touch("a")
	plan.Touch("b")

	assert.Equal(t, []string{"a", "b", "c"}, plan.Dirty())
}

func TestDirty_IsEmptyForAPlanThatChangedNothing(t *testing.T) {
	plan := tree.New(level("", "a", "b"))

	assert.Empty(t, plan.Dirty())
}

func TestReparented_ARenumberIsNotAReparent(t *testing.T) {
	plan := tree.New(level("", "a", "b"))

	plan.Reorder("", "b", 0)

	assert.Equal(t, []string{"a", "b"}, plan.Dirty(), "both rows took a new index")
	assert.False(t, plan.Reparented("a"), "neither of them changed level")
	assert.False(t, plan.Reparented("b"))
}

func TestReparented_ReportsARowThatChangedContainer(t *testing.T) {
	plan := tree.New([]tree.Node{{ID: "a"}, {ID: "b"}})

	plan.SetParent("b", "a")

	assert.True(t, plan.Reparented("b"))
	assert.False(t, plan.Reparented("a"))
}

func TestReparented_IgnoresAMoveOntoTheSameContainer(t *testing.T) {
	plan := tree.New(level("", "a", "b"))

	plan.SetParent("b", "")

	assert.False(t, plan.Reparented("b"))
}

func TestReparented_CoversTheChildrenAReparentPromoted(t *testing.T) {
	plan := tree.New([]tree.Node{{ID: "f"}, {ID: "a", ParentID: "f"}})

	plan.Reparent("f", "")

	assert.True(t, plan.Reparented("a"))
}

func TestReparented_CoversAnAddedRow(t *testing.T) {
	plan := tree.New(nil)

	plan.Add(tree.Node{ID: "f", ParentID: "c1"})

	assert.True(t, plan.Reparented("f"))
}

func TestReparented_ForgetsADroppedRow(t *testing.T) {
	plan := tree.New([]tree.Node{{ID: "a"}, {ID: "b"}})
	plan.SetParent("b", "a")

	plan.Drop("b")

	assert.False(t, plan.Reparented("b"))
	assert.Empty(t, plan.Dirty())
}

func TestReparented_IsNotSetByTouch(t *testing.T) {
	plan := tree.New(level("", "a"))

	plan.Touch("a")

	assert.Equal(t, []string{"a"}, plan.Dirty())
	assert.False(t, plan.Reparented("a"))
}

func TestReparented_IsFalseForARowTheForestNeverHeld(t *testing.T) {
	plan := tree.New(level("", "a"))

	assert.False(t, plan.Reparented("missing"))
}
