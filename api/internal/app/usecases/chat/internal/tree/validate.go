package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The checks every move passes before anything is written. Folder ops
// (repo-wide, no workspace of their own) and chat ops (workspace-scoped) carry
// different container rules, so each gets its own pair below.

// checkFolderMove refuses a move onto a container that does not exist, lies
// inside the moved row's own subtree, would give it a parent outside its own
// repo scope, or would cross it into a DIFFERENT context within that same
// repo (the finer half of the golden rule — see checkFolderContextMove's own
// doc). The context check runs only for a move, never a create: a brand new
// folder has no prior position to have crossed away from — it simply takes
// on whatever context its chosen parent already implies.
func (u *chatFolderUsecase) checkFolderMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	folderRepoID string,
	id string,
	destination string,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkFolderContainer(ctx, snapshot, folderRepoID, destination); err != nil {
		return err
	}
	if snapshot.plan.Reaches(destination, id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	if err := u.checkFolderContextMove(ctx, snapshot, id, destination); err != nil {
		return err
	}
	return nil
}

// checkFolderContextMove is the golden rule's fine grain: "context", in
// order, is Project -> Repo -> Locked branch -> Parent unlocked (branch) —
// four different anchors a folder can sit under, not just two (home vs a
// repo). A folder may move freely among its own siblings and descendants
// WITHIN whatever anchor it already sits under, but never jump to a
// different one — a different branch's own subtree, the bare repo root
// versus any branch, or project-home versus a repo — even though
// checkFolderContainer's repo-level check alone would allow all of those
// (same repo, or both "").
//
// The anchor itself is never stored: it is answered by walking ParentID
// until a row with a real WorkspaceID turns up (a locked or unlocked
// branch's own owning row) — reaching the root without finding one means
// the anchor is the bare repo root (or project home, already distinguished
// by folderRepoID's own "" convention). Comparing this walk's answer for the
// folder's CURRENT position against the same walk for its PROPOSED one is
// the whole check; nothing here is written or persisted.
func (u *chatFolderUsecase) checkFolderContextMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
	destination string,
) error {
	from, err := u.nearestWorkspaceAnchor(ctx, snapshot, id)
	if err != nil {
		return fmt.Errorf("agent chat folder: move %s: %w", id, err)
	}
	to, err := u.nearestWorkspaceAnchor(ctx, snapshot, destination)
	if err != nil {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, err)
	}
	if from != to {
		return fmt.Errorf(
			"agent chat folder: move %s under %s crosses context: %w",
			id, destination, ErrCrossContext,
		)
	}
	return nil
}

// nearestWorkspaceAnchor walks id's ParentID chain (id's OWN row included)
// until a row that genuinely OWNS a workspace turns up, and answers that
// workspace's id. "" is a real answer, not "not found": it means the walk
// reached the panel root without ever crossing an owning row — the bare repo
// root, or project home.
//
// "Owns" is answered by ownsWorkspace, deliberately NOT "carries a non-empty
// WorkspaceID" — a plain chat carries one too (domain.Chat's own doc:
// "WorkspaceID always resolves to a repo", populated on every row regardless
// of Type), but that is the chat's GROUND, the workspace whose API scope it
// happens to live under, not a context boundary; an unlocked branch's own
// owning row is ALSO merely ChatTypeChat (owningChatType only promotes a
// locked branch, a repo home or a project home to ChatTypeBranch), so Type
// alone cannot tell the two apart either. Stopping at "carries a
// WorkspaceID" made a folder sitting at the bare root (whose own walk
// correctly falls through folders, which never carry WorkspaceID, straight
// to "") compare unequal against a PLAIN CHAT SIBLING at that identical
// root — the chat's walk stopped one step early, on itself, and reported its
// own ground workspace as if it were a branch anchor. Two rows at the exact
// same position in the exact same context reported different anchors purely
// because one kind (chat) happens to store a WorkspaceID pointer the other
// kind (folder) never does — caught live as a folder that could reorder past
// a chat sibling but could never be filed INTO one, refused with "crosses
// context" for two rows that had never left it.
func (u *chatFolderUsecase) nearestWorkspaceAnchor(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (string, error) {
	seen := map[string]bool{}
	for id != "" {
		if seen[id] {
			return "", nil
		}
		seen[id] = true
		row := snapshot.row(id)
		if row == nil {
			got, err := u.chats.Get(ctx, id)
			if err != nil {
				return "", fmt.Errorf("resolve %s: %w", id, err)
			}
			row = &got
		}
		if row.WorkspaceID != "" {
			owns, err := u.ownsWorkspace(ctx, *row)
			if err != nil {
				return "", fmt.Errorf("resolve owner of workspace %s: %w", row.WorkspaceID, err)
			}
			if owns {
				return row.WorkspaceID, nil
			}
		}
		id = row.ParentID
	}
	return "", nil
}

// ownsWorkspace answers whether row is the one row that OWNS row.WorkspaceID
// — the same "which row addresses this workspace" question the sidebar
// itself asks (ResolveOwningChat, owning_rows.go), reused here rather than
// re-derived: a row's own WorkspaceID pointer is not enough on its own (see
// nearestWorkspaceAnchor's doc), since any ordinary chat started inside a
// workspace carries the identical pointer its owning row does.
func (u *chatFolderUsecase) ownsWorkspace(ctx context.Context, row domain.Chat) (bool, error) {
	rows, err := u.chats.ListByWorkspace(ctx, row.WorkspaceID)
	if err != nil {
		return false, err
	}
	owner, ok := ResolveOwningChat(rows)
	return ok && owner.ID == row.ID, nil
}

// checkFolderContainer validates a folder's parent id: "" is the panel root
// (always legal — nothing to inherit a scope FROM, and folderRepoID is
// already this folder's own fixed scope, set once at creation and never
// rewritten by a move), and anything else must be a row that exists AND
// share folderRepoID.
//
// The scoping golden rule: a folder may only be filed under a parent whose
// own scope is the SAME one this folder already has — "" (project-home) with
// "", a repo id only with that same repo id. This is what makes ListInRepo's
// filter (tree.go) actually TRUE rather than merely asserted: nothing can
// reach a repo's tree, or home's, that does not already carry that repo's id.
// Caught live as a folder or chat left "on top of" the wrong repo after being
// dragged there.
func (u *chatFolderUsecase) checkFolderContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	folderRepoID string,
	parentID string,
) error {
	if parentID == "" {
		return nil
	}
	row := snapshot.row(parentID)
	if row == nil {
		got, err := u.chats.Get(ctx, parentID)
		if err != nil {
			return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
		}
		row = &got
	}
	if row.Type == nodePhantomType {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, ErrNotAContainer)
	}
	parentRepoID, err := u.repoScopeOf(ctx, *row)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	if parentRepoID != nil && *parentRepoID != folderRepoID {
		return fmt.Errorf(
			"agent chat folder: parent %s belongs to repo %q, not %q: %w",
			parentID, *parentRepoID, folderRepoID, ErrCrossRepo,
		)
	}
	return nil
}

// repoScopeOf answers a row's own repo scope, or nil when the row carries
// none to check against (a plain chat bubble with no workspace of its own —
// nothing to conflict with, same posture checkParentKind already takes for a
// chat parent in the identical situation).
//
// A FOLDER answers its own stored RepoID directly — no lookup. A row that
// OWNS a workspace (a BRANCH, or a CHAT forked with its own worktree)
// resolves it via WorkspaceGitStatus.RepoOf, the one case that needs the
// workspace layer at all.
func (u *chatFolderUsecase) repoScopeOf(ctx context.Context, row domain.Chat) (*string, error) {
	if row.Type == domain.ChatTypeFolder {
		id := row.RepoID
		return &id, nil
	}
	if row.WorkspaceID == "" {
		return nil, nil
	}
	id, err := u.workspaces.RepoOf(ctx, row.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// checkChatMove refuses a chat move onto a container that does not exist,
// belongs to another workspace, or lies inside the moved chat's own subtree.
//
// ownWorktree is threaded straight from placeChat: true only for its
// own-worktree-creation caller, createOwnWorktreeChat (see placeChat's doc
// comment) — never for an ordinary move, which always passes false.
func (u *chatFolderUsecase) checkChatMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	id string,
	destination string,
	ownWorktree bool,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkChatContainer(ctx, snapshot, workspaceID, destination, ownWorktree); err != nil {
		return err
	}
	if snapshot.plan.Reaches(destination, id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	return nil
}

// checkChatContainer validates a chat's parent id: "" is the panel root, a
// FOLDER or a BRANCH is accepted unconditionally (neither carries a workspace
// to conflict with — a branch row is a process boundary, not a workspace one),
// and a CHAT must belong to workspaceID — unless ownWorktree names an
// own-worktree creation forking off a chat that already owns a worktree of its
// own, which is legal for the same reason forking off a BRANCH row is (see
// checkParentKind). A row that resolves to a DIFFERENT workspace is reported
// as a cross-workspace edge rather than as a missing row, because the two are
// fixed in different ways.
func (u *chatFolderUsecase) checkChatContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	parentID string,
	ownWorktree bool,
) error {
	if parentID == "" {
		return nil
	}
	if row := snapshot.row(parentID); row != nil {
		return checkParentKind(*row, workspaceID, parentID, ownWorktree)
	}
	row, err := u.chats.Get(ctx, parentID)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	return checkParentKind(row, workspaceID, parentID, ownWorktree)
}

// checkParentKind is the second half of checkChatContainer, split out because
// both the snapshot-membership path and the keyed-lookup fallback answer the
// same question about the row once they have it.
//
// The keyed read heals the chat read model for the one id it was asked about;
// the workspace list only heals a model that is entirely empty — so the
// authoritative answer here can name a row the snapshot's list did not carry.
// Refusing it would reject a drop onto a chat the user can see.
//
// ownWorktree is true only for an own-worktree CREATION (never a move, and
// never an ordinary thread): forking a new workspace off a row that already
// carries one is legal regardless of that row's kind, exactly as it already is
// off a BRANCH row — the new chat has no workspace yet to conflict with
// anything, so a worktree-owning CHAT parent is accepted the same way a
// FOLDER or BRANCH parent is. An ordinary thread still enforces the
// same-workspace rule against a CHAT parent: it inherits that parent's cwd, so
// one claiming a different workspace than its parent is meaningless.
func checkParentKind(
	row domain.Chat,
	workspaceID string,
	parentID string,
	ownWorktree bool,
) error {
	if row.Type == nodePhantomType {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, ErrNotAContainer)
	}
	if row.Type == domain.ChatTypeFolder || row.Type == domain.ChatTypeBranch {
		return nil
	}
	if ownWorktree && row.WorkspaceID != "" {
		return nil
	}
	if row.WorkspaceID == workspaceID {
		return nil
	}
	return fmt.Errorf(
		"agent chat folder: parent %s belongs to workspace %s, not %s: %w",
		parentID, row.WorkspaceID, workspaceID, ErrCrossWorkspace,
	)
}
