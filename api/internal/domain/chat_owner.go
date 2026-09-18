package domain

// SharedGround reports whether many conversations legitimately run in w
// without any of them owning it: the repo's default checkout, a locked
// branch, or the project home. An ordinary fork is private ground — the
// earliest row anchored to it is, by construction, the one that forked it.
func (w Workspace) SharedGround() bool {
	return w.IsDefault || w.RepoID == "" || w.RendersAsBranch()
}

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
//
// sharedGround is Workspace.SharedGround for the workspace the rows hold:
// there a titled legacy row is a user's conversation, never a minted owner;
// on private ground (an ordinary fork) the conversation that forked the
// branch has been chatted in, and is still its owner.
func ResolveOwningChat(
	rows []Chat,
	sharedGround bool,
) (Chat, bool) {
	candidates := ownerCandidates(rows, sharedGround)
	if len(candidates) == 0 {
		return Chat{}, false
	}
	owner := candidates[0]
	for _, row := range candidates[1:] {
		if !preferredOwner(owner, row) {
			owner = row
		}
	}
	return owner, true
}

// ownerCandidates narrows rows to the ones that can own the workspace: every
// row that RECORDS ownership when any does, otherwise every legacy row that
// is not provably a thread — one filed under the workspace's own row or under
// another row of the same workspace was created inside it, never for it. On
// shared ground a titled conversation is excluded too.
func ownerCandidates(
	rows []Chat,
	sharedGround bool,
) []Chat {
	sameWorkspace := make(map[string]bool, len(rows))
	for _, row := range rows {
		sameWorkspace[row.ID] = true
	}
	var recorded, legacy []Chat
	for _, row := range rows {
		switch {
		case row.OwnsWorkspace:
			recorded = append(recorded, row)
		case row.Type == ChatTypeBranch:
			legacy = append(legacy, row)
		case row.WorkspaceID != "" && row.ParentID == row.WorkspaceID:
			continue
		case row.ParentID != "" && sameWorkspace[row.ParentID]:
			continue
		case sharedGround && (row.Title != "" || row.TitleLocked):
			continue
		default:
			legacy = append(legacy, row)
		}
	}
	if len(recorded) > 0 {
		return recorded
	}
	return legacy
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
