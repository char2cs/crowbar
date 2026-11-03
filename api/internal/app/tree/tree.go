// Package tree plans a sibling-ordered forest of rows carrying only a parent id and a dense index. It
// is ignorant of why a parent changed, since callers disagree (lineage vs organisation), and pure
// arithmetic over one in-memory snapshot, so callers plan once and write only what moved.
package tree

// Tree is a plan in progress over one forest: built from rows as read, mutated by the operations
// below, read back through Dirty. One plan belongs to one goroutine, so it needs no locking.
type Tree interface {
	// Node returns a row as the plan currently has it, and whether the forest
	// holds it at all. The bool is the existence answer callers need before
	// treating an id as a container.
	Node(
		id string,
	) (Node, bool)
	// Members returns a container's sibling space in render order; "" is the root, and ids never collide
	// across row kinds, so one membership rule covers every level.
	Members(
		container string,
	) []Node
	// NextSlot returns the end-of-level index a row joining container takes; a new row must ask, since
	// a zero index collides with slot 0 and surfaces at the top.
	NextSlot(
		container string,
	) int
	// IndexOf returns a row's current position in a sibling space, or 0 when the
	// level does not hold it — the shape a caller produces when it reorders a row
	// it has just placed somewhere else.
	IndexOf(
		container string,
		id string,
	) int
	// Add puts a new row into the forest and marks it changed, since a row the
	// plan invented has by definition never been written.
	Add(
		node Node,
	)
	// Drop removes a row from the forest and from the changed set: a row that is
	// going away must not also be reported as a row to write.
	Drop(
		id string,
	)
	// SetParent moves a row into another container, marking it changed only when the container differs.
	// The tree is told the destination, not the reason.
	SetParent(
		id string,
		parentID string,
	)
	// Touch marks a row changed for a reason the tree does not model (the caller's own edge moved), so the
	// write is not dropped from the plan.
	Touch(
		id string,
	)
	// Reorder densifies container's whole sibling space to 0..n-1, optionally inserting placed at target
	// first (target < 0 just densifies). Dense levels keep the next drop index meaningful.
	Reorder(
		container string,
		placed string,
		target int,
	)
	// Reparent hands every direct child of id to destination without touching id, so a container can be
	// emptied before it is dropped. Callers send differently-constrained rows elsewhere first.
	Reparent(
		id string,
		destination string,
	)
	// Reaches reports whether walking up from container arrives at ancestor — the cycle test every move
	// owes. A visited set bounds the walk on an already corrupt edge.
	Reaches(
		container string,
		ancestor string,
	) bool
	// Dirty returns the ids whose placement the plan changed, sorted, so a caller
	// whose writes fail halfway fails reproducibly rather than arbitrarily.
	Dirty() []string
	// Reparented reports whether the plan moved this row to a different container rather than renumbering
	// it in place. Field-level stores must write only the index for the latter: restating the parent from
	// a possibly stale snapshot could undo a concurrent move.
	Reparented(
		id string,
	) bool
}

// New builds a plan over rows already read; they are copied so the caller's slice is never renumbered.
// The id→index map keeps renumbering linear instead of quadratic in row count.
func New(
	nodes []Node,
) Tree {
	t := &siblingTree{
		nodes:      make([]Node, len(nodes)),
		at:         make(map[string]int, len(nodes)),
		dirty:      map[string]bool{},
		reparented: map[string]bool{},
	}
	copy(t.nodes, nodes)
	for i, node := range t.nodes {
		t.at[node.ID] = i
	}
	return t
}
