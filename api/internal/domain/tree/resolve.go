package tree

func ResolveWorkspace(t Tree, id string, f Facts) string {
	for seen := map[string]bool{}; id != "" && !seen[id]; {
		seen[id] = true
		n, ok := t.Nodes[id]
		if !ok {
			break
		}
		switch n.Kind {
		case KindWorkspace:
			return n.ID
		case KindRepo:
			return f.RepoHome[n.ID]
		case KindChat:
			if ws := f.BoundTo[n.ID]; ws != "" {
				return ws
			}
		case KindFolder:
		}
		id = n.ParentID
	}
	return f.ProjectHome
}
