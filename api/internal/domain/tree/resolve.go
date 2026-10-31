package tree

// ResolveWorkspace is the workspace a row placed directly under id runs in: a
// workspace node's own, a chat's bound one, a repo's home checkout, else the
// project home. Folders are transparent.
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
