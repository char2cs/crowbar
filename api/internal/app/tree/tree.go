// Package tree plans a sibling-ordered forest: rows that carry a parent id and a
// dense index within that parent's sibling space, and nothing else.
//
// It is deliberately ignorant of WHY a parent changed, because its callers do
// not agree on that and never will. The workspace sidebar must never let a drag
// rewrite git lineage — organisation and lineage are separate edges there, and
// the tree is only ever told the organisational one. The chats panel has a
// single edge, so its drag is required to rewrite the very thing the sidebar
// protects. A core that encoded either rule would have to be fought by the other
// caller, so it encodes neither: a caller resolves its own edges down to one
// parent id, hands the rows over, and reads back the ids that moved.
//
// Nothing here takes a context, reaches a store, or returns an error. Planning
// is arithmetic over rows already in memory, which is what lets a caller read
// the world ONCE, plan the whole change, and only then write what actually
// moved. That is not merely an optimisation where the rows come from an
// asynchronous projection: a re-read taken between a write and the renumber
// that follows it can still be serving the pre-write list, and planning from a
// single snapshot removes that race instead of papering over it with a barrier.
package tree

// Tree is a plan in progress over one sibling-ordered forest. It is built from
// the rows as they were read, mutated by the operations below, and finally read
// back through Dirty — the ids whose placement the plan changed, and only those.
//
// It is a plain in-memory value with no synchronisation: one plan belongs to one
// operation on one goroutine, which is also what makes it safe to mutate rows in
// place while guards run against them.
type Tree interface {
	// Node returns a row as the plan currently has it, and whether the forest
	// holds it at all. The bool is the existence answer callers need before
	// treating an id as a container.
	Node(
		id string,
	) (Node, bool)
	// Members returns a container's sibling space in render order. A container id
	// is any node's id, or "" for the root; ids never collide across whatever row
	// kinds a caller merges into one forest, so a single membership rule covers
	// every level.
	Members(
		container string,
	) []Node
	// NextSlot returns the index a row joining container would take — the end of
	// that sibling space. A newly created row has to ask for one: every kind of
	// row shares a level, so a row that keeps the zero value collides with
	// whatever already holds slot 0 and surfaces at the TOP of a level it should
	// have joined at the end.
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
	// SetParent moves a row into another container, marking it changed only when
	// the container actually differs. The tree is told the destination and
	// nothing about the reason, so the same call serves a caller that treats a
	// drag as pure organisation and one that treats it as a real re-parenting.
	SetParent(
		id string,
		parentID string,
	)
	// Touch records a row as changed for a reason the tree does not model — the
	// caller's own edge behind the parent id moved while the container itself did
	// not. Without it that write is silently dropped from the plan and the row
	// reverts on the next read.
	Touch(
		id string,
	)
	// Reorder assigns dense 0..n-1 indices across container's WHOLE sibling
	// space, optionally lifting placed out and re-inserting it at target first. A
	// target below zero means "placed is not being positioned here", which is how
	// a caller densifies the level a row has just left. Leaving every level dense
	// after every move is what makes the next drop index mean what it says.
	Reorder(
		container string,
		placed string,
		target int,
	)
	// Reparent hands every direct child of id to destination without touching id
	// itself, so a caller can empty a container before dropping it rather than
	// cascading the delete through rows the user only meant to unfile.
	//
	// It takes ONE destination on purpose. A caller whose rows are not all alike
	// may need to send some of them elsewhere — a row kind whose expressible
	// containers are narrower than the forest's cannot always follow its
	// siblings — and it does that by moving those rows itself first; whatever is
	// still a child when this runs follows destination.
	Reparent(
		id string,
		destination string,
	)
	// Reaches reports whether walking UP from container ever arrives at ancestor.
	// It is the cycle test every move owes: a level that ends up inside its own
	// subtree still exists, nothing renders it, and nothing can drag it back out.
	// The walk is bounded by a visited set so an already corrupt edge is answered
	// rather than spun on.
	Reaches(
		container string,
		ancestor string,
	) bool
	// Dirty returns the ids whose placement the plan changed, sorted, so a caller
	// whose writes fail halfway fails reproducibly rather than arbitrarily.
	Dirty() []string
	// Reparented reports whether the plan moved this row into a DIFFERENT
	// container, as against merely renumbering it where it already sat.
	//
	// Dirty alone cannot tell a caller that, and the difference decides what a
	// write may ASSERT. A densify decides nothing about any row's container, so a
	// caller whose store takes field-level writes must write only the index for
	// those rows: restating the parent restates it from the snapshot, and a
	// snapshot read from an asynchronous projection can still be serving the
	// container a row had before the operation just before this one moved it.
	Reparented(
		id string,
	) bool
}

// New builds a plan over rows already read from wherever they live. The rows are
// copied, so a caller's slice is never renumbered behind its back.
//
// The id→index map is built up front because a renumber touches every row at a
// level and each touch resolves a row by id; without it the walk is quadratic in
// the forest's row count, which is exactly the shape that looks fine on a demo
// repo and stalls on a real one.
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
