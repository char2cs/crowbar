// Package cascade computes the deletion order for a workspace subtree: locked nodes
// are omitted but their unlocked descendants are still included.
package cascade

// Node is a workspace entry in the hierarchy tree; Parent is "" for the root.
type Node struct {
	ID     string
	Parent string
	Locked bool
}

// Plan returns the deepest-first deletion order of the subtree rooted at rootID,
// skipping locked nodes but descending into them; empty when rootID is not in all.
func Plan(
	rootID string,
	all []Node,
) []string {
	index := buildIndex(all)
	if _, ok := index[rootID]; !ok {
		return nil
	}
	p := planner{children: buildChildren(all), visited: map[string]bool{}}
	p.dfs(rootID, index)
	return p.order
}

type planner struct {
	children map[string][]string
	order    []string
	visited  map[string]bool
}

func (p *planner) dfs(
	id string,
	index map[string]Node,
) {
	if p.visited[id] {
		return
	}
	p.visited[id] = true
	for _, childID := range p.children[id] {
		p.dfs(childID, index)
	}
	n := index[id]
	if n.Locked {
		return
	}
	p.order = append(p.order, id)
}

func buildIndex(
	all []Node,
) map[string]Node {
	m := make(map[string]Node, len(all))
	for _, n := range all {
		m[n.ID] = n
	}
	return m
}

func buildChildren(
	all []Node,
) map[string][]string {
	m := make(map[string][]string, len(all))
	for _, n := range all {
		if n.Parent != "" {
			m[n.Parent] = append(m[n.Parent], n.ID)
		}
	}
	return m
}
