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

// Ancestors is the chat lineage above node id, nearest first: every chat node
// above it, and each workspace node as its owning chat (ownerOf, "" for none).
// Folders are stepped through; a repo or the project root ends it.
func Ancestors(t Tree, id string, ownerOf func(workspaceID string) string) []string {
	var out []string
	seen := map[string]bool{id: true}
	n, ok := t.Nodes[id]
	for ok && n.ParentID != "" && !seen[n.ParentID] {
		seen[n.ParentID] = true
		if n, ok = t.Nodes[n.ParentID]; !ok || n.Kind == KindRepo {
			break
		}
		switch n.Kind {
		case KindChat:
			out = append(out, n.ID)
		case KindWorkspace:
			if owner := ownerOf(n.ID); owner != "" {
				out = append(out, owner)
			}
		case KindFolder, KindRepo:
		}
	}
	return out
}
