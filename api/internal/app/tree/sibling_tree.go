package tree

import "slices"

type siblingTree struct {
	nodes []Node
	at    map[string]int
	dirty map[string]bool
}

func (t *siblingTree) Node(
	id string,
) (Node, bool) {
	at, ok := t.at[id]
	if !ok {
		return Node{}, false
	}
	return t.nodes[at], true
}

func (t *siblingTree) Members(
	container string,
) []Node {
	rows := make([]Node, 0, len(t.nodes))
	for _, node := range t.nodes {
		if node.ParentID == container {
			rows = append(rows, node)
		}
	}
	slices.SortFunc(rows, compareNodes)
	return rows
}

func (t *siblingTree) NextSlot(
	container string,
) int {
	slots := 0
	for _, node := range t.nodes {
		if node.ParentID == container {
			slots++
		}
	}
	return slots
}

func (t *siblingTree) IndexOf(
	container string,
	id string,
) int {
	at := slices.IndexFunc(t.Members(container), func(node Node) bool { return node.ID == id })
	return max(at, 0)
}

func (t *siblingTree) Add(
	node Node,
) {
	t.at[node.ID] = len(t.nodes)
	t.nodes = append(t.nodes, node)
	t.dirty[node.ID] = true
}

func (t *siblingTree) Drop(
	id string,
) {
	if _, ok := t.at[id]; !ok {
		return
	}
	delete(t.dirty, id)
	t.nodes = slices.DeleteFunc(t.nodes, func(node Node) bool { return node.ID == id })
	t.at = make(map[string]int, len(t.nodes))
	for i, node := range t.nodes {
		t.at[node.ID] = i
	}
}

func (t *siblingTree) SetParent(
	id string,
	parentID string,
) {
	at, ok := t.at[id]
	if !ok || t.nodes[at].ParentID == parentID {
		return
	}
	t.nodes[at].ParentID = parentID
	t.dirty[id] = true
}

func (t *siblingTree) Touch(
	id string,
) {
	if _, ok := t.at[id]; !ok {
		return
	}
	t.dirty[id] = true
}

func (t *siblingTree) Reorder(
	container string,
	placed string,
	target int,
) {
	rows := t.Members(container)
	if placed != "" && target >= 0 {
		rows = reinsert(rows, placed, target)
	}
	for i, row := range rows {
		t.setOrder(row.ID, i)
	}
}

func (t *siblingTree) Reparent(
	id string,
	destination string,
) {
	for _, child := range t.Members(id) {
		t.SetParent(child.ID, destination)
	}
}

func (t *siblingTree) Reaches(
	container string,
	ancestor string,
) bool {
	visited := map[string]bool{}
	for at := container; at != "" && !visited[at]; at = t.parentOf(at) {
		if at == ancestor {
			return true
		}
		visited[at] = true
	}
	return false
}

func (t *siblingTree) Dirty() []string {
	ids := make([]string, 0, len(t.dirty))
	for id := range t.dirty {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (t *siblingTree) setOrder(
	id string,
	order int,
) {
	at, ok := t.at[id]
	if !ok || t.nodes[at].Order == order {
		return
	}
	t.nodes[at].Order = order
	t.dirty[id] = true
}

func (t *siblingTree) parentOf(
	id string,
) string {
	at, ok := t.at[id]
	if !ok {
		return ""
	}
	return t.nodes[at].ParentID
}

func reinsert(
	rows []Node,
	id string,
	target int,
) []Node {
	at := slices.IndexFunc(rows, func(node Node) bool { return node.ID == id })
	if at < 0 {
		return rows
	}
	moved := rows[at]
	rows = slices.Delete(rows, at, at+1)
	return slices.Insert(rows, min(max(target, 0), len(rows)), moved)
}
