// Package cascade computes the deletion order for a workspace subtree,
// applying the skip-locked rule from 07 §5: locked nodes are omitted
// from the result but their unlocked descendants are still included.
package cascade

// Node is a workspace entry in the hierarchy tree.
// Parent is the empty string for the root of the tree.
// Locked nodes are excluded from the deletion plan but their descendants
// are still visited and included when unlocked.
type Node struct {
	ID     string
	Parent string
	Locked bool
}

// Plan returns the deletion order (deepest-first, post-order DFS) of the
// subtree rooted at rootID, skipping locked nodes but still descending
// into their children. Returns an empty slice when rootID is not found
// in all.
func Plan(
	rootID string,
	all []Node,
) []string {
	index := buildIndex(all)
	if _, ok := index[rootID]; !ok {
		return nil
	}
	p := planner{children: buildChildren(all)}
	p.dfs(rootID, index)
	return p.order
}

type planner struct {
	children map[string][]string
	order    []string
}

func (p *planner) dfs(
	id string,
	index map[string]Node,
) {
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
