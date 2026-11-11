package domain

// ResolveOwningChat picks a workspace's owner from its candidate chat rows: the row
// recording Chat.OwnsWorkspace, the earliest if several do, so every surface agrees.
// No recording row means no owner; there is no heuristic fallback.
func ResolveOwningChat(
	rows []Chat,
) (Chat, bool) {
	var owner Chat
	found := false
	for _, row := range rows {
		if !row.OwnsWorkspace {
			continue
		}
		if !found || earlier(row, owner) {
			owner, found = row, true
		}
	}
	return owner, found
}

func earlier(a, b Chat) bool {
	if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
		return c < 0
	}
	return a.ID < b.ID
}
