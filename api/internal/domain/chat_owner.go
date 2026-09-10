package domain

// ResolveOwningChat picks the chat that owns a workspace from its candidate
// rows — typically everything ListChatsByWorkspace(workspaceID) returns — for
// every wire-facing surface that still addresses a workspace's worktree
// through a CHAT id (chat-scoped HTTP routes, WorkspaceDTO.OwningChatID): a
// worktree is many-chats-to-one, since every ordinary conversation started
// inside a workspace carries that same workspace id, so several candidate
// rows legitimately share one WorkspaceID.
//
// A legacy ChatTypeBranch row — from before 2026-09-08
// sidebar-placement-unification Task 9 retired the machinery that minted
// them — always wins over an ordinary conversation sharing its workspace:
// that row was deliberately minted BESIDE a chatted-in conversation rather
// than replacing it, specifically so it would be the one addressed. Every
// row minted going forward is ChatTypeChat, so among those the earliest one
// wins — the workspace's own owning row is always minted before the
// workspace itself (see the chat-first create in usecases/chat's
// MintOwningChat), so it is always the earliest candidate for its
// WorkspaceID.
func ResolveOwningChat(
	rows []Chat,
) (Chat, bool) {
	if len(rows) == 0 {
		return Chat{}, false
	}
	owner := rows[0]
	for _, row := range rows[1:] {
		if !preferredOwner(owner, row) {
			owner = row
		}
	}
	return owner, true
}

// preferredOwner reports whether held keeps the workspace against
// challenger, mirroring the tiebreak the (now-deleted) boot backfill used to
// enforce so a legacy branch row still resolves exactly as it always did.
func preferredOwner(
	held Chat,
	challenger Chat,
) bool {
	if (held.Type == ChatTypeBranch) != (challenger.Type == ChatTypeBranch) {
		return held.Type == ChatTypeBranch
	}
	if c := held.CreatedAt.Compare(challenger.CreatedAt); c != 0 {
		return c < 0
	}
	return held.ID < challenger.ID
}
