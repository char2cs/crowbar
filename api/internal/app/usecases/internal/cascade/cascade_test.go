package cascade

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlan_DeepestFirstSkippingLocked(t *testing.T) {
	all := []Node{
		{ID: "root", Parent: ""},
		{ID: "a", Parent: "root"},
		{ID: "b", Parent: "a", Locked: true},
		{ID: "c", Parent: "b"},
	}
	order := Plan("root", all)
	// c and a and root deletable; b skipped; deepest-first
	assert.Equal(t, []string{"c", "a", "root"}, order)
}

func TestPlan_LockedRootYieldsOnlyUnlockedDescendants(t *testing.T) {
	all := []Node{
		{ID: "root", Parent: "", Locked: true},
		{ID: "a", Parent: "root"},
	}
	assert.Equal(t, []string{"a"}, Plan("root", all))
}

func TestPlan_UnknownRootEmpty(t *testing.T) {
	assert.Empty(t, Plan("nope", nil))
}

func TestPlan_SingleUnlockedNode(t *testing.T) {
	all := []Node{
		{ID: "root", Parent: ""},
	}
	assert.Equal(t, []string{"root"}, Plan("root", all))
}

func TestPlan_SingleLockedNode(t *testing.T) {
	all := []Node{
		{ID: "root", Parent: "", Locked: true},
	}
	assert.Empty(t, Plan("root", all))
}

func TestPlan_AllLockedYieldsEmpty(t *testing.T) {
	all := []Node{
		{ID: "root", Parent: "", Locked: true},
		{ID: "a", Parent: "root", Locked: true},
		{ID: "b", Parent: "a", Locked: true},
	}
	assert.Empty(t, Plan("root", all))
}

func TestPlan_DeepestFirstOrder(t *testing.T) {
	// root -> a -> b (all unlocked)
	all := []Node{
		{ID: "root", Parent: ""},
		{ID: "a", Parent: "root"},
		{ID: "b", Parent: "a"},
	}
	order := Plan("root", all)
	assert.Equal(t, []string{"b", "a", "root"}, order)
}

func TestPlan_MultipleChildrenDeepestFirst(t *testing.T) {
	// root -> a, root -> b; a -> c; all unlocked
	all := []Node{
		{ID: "root", Parent: ""},
		{ID: "a", Parent: "root"},
		{ID: "b", Parent: "root"},
		{ID: "c", Parent: "a"},
	}
	order := Plan("root", all)
	// c must appear before a; a and b must appear before root
	assert.Equal(t, "root", order[len(order)-1])
	assert.Contains(t, order, "c")
	assert.Contains(t, order, "a")
	assert.Contains(t, order, "b")
	// c before a
	cIdx, aIdx := index(order, "c"), index(order, "a")
	assert.Less(t, cIdx, aIdx)
}

func index(
	s []string,
	v string,
) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func TestPlan_SelfCycleTerminatesYieldingNodeOnce(t *testing.T) {
	// A corrupt self-loop (node is its own parent) must not stack-overflow; the
	// node is processed at most once.
	all := []Node{{ID: "root", Parent: "root"}}
	order := Plan("root", all)
	assert.Equal(t, []string{"root"}, order)
}

func TestPlan_TwoNodeCycleTerminates(t *testing.T) {
	// a -> b -> a corrupt 2-cycle. Walk terminates and yields each node once.
	all := []Node{
		{ID: "a", Parent: "b"},
		{ID: "b", Parent: "a"},
	}
	order := Plan("a", all)
	assert.ElementsMatch(t, []string{"a", "b"}, order)
	assert.Len(t, order, 2)
}

func TestPlan_UnlockedDescendantUnderLockedAncestor(t *testing.T) {
	// locked middle node; its unlocked child still included
	all := []Node{
		{ID: "root", Parent: ""},
		{ID: "mid", Parent: "root", Locked: true},
		{ID: "leaf", Parent: "mid"},
	}
	order := Plan("root", all)
	assert.Contains(t, order, "leaf")
	assert.Contains(t, order, "root")
	assert.NotContains(t, order, "mid")
	// leaf before root
	assert.Less(t, index(order, "leaf"), index(order, "root"))
}
