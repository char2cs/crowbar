package domain

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
