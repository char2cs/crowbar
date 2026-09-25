package domain

// ResolveOwningChat picks the chat that owns a workspace from its candidate
// rows — typically everything ListChatsByWorkspace(workspaceID) returns. A
// worktree is many-chats-to-one (every conversation started inside a workspace
// carries its id), so the owner is the row that RECORDS ownership: the chat
// the chat-first create minted and attached (Chat.OwnsWorkspace), which the
// boot reconcile guarantees every live workspace has (invariant D4).
//
// Should more than one row ever record it, the earliest wins, so every surface
// names the same owner. No row recording ownership means no owner: there is no
// heuristic fallback any more — all data is on the recorded model (spec §6.3).
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
