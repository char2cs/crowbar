package tree

type Tree interface {
	Node(
		id string,
	) (Node, bool)
	Members(
		container string,
	) []Node
	NextSlot(
		container string,
	) int
	IndexOf(
		container string,
		id string,
	) int
	Add(
		node Node,
	)
	Drop(
		id string,
	)
	SetParent(
		id string,
		parentID string,
	)
	Touch(
		id string,
	)
	Reorder(
		container string,
		placed string,
		target int,
	)
	Reparent(
		id string,
		destination string,
	)
	Reaches(
		container string,
		ancestor string,
	) bool
	Dirty() []string
	Reparented(
		id string,
	) bool
}

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
