package tree

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The cascade that takes a chat's threads and worktrees with it.

// DeleteChat erases chatID and every chat threaded below it, together with any
// worktree that subtree was the last thing holding. A folder id is refused as
// not-found — the folder verb promotes what it held, this one cascades.
//
// Worktrees are reaped before any chat is purged: a chat is the only handle
// on its worktree, so a failed reap must fail (and return, not log) the whole
// delete rather than purge the chat and orphan the directory on disk. Without
// consent, work at risk in any of them refuses the delete before anything goes.
func (u *chatFolderUsecase) DeleteChat(
	ctx context.Context,
	chatID string,
	consent domain.DeleteConsent,
) (ChatDeletion, error) {
	current, err := u.chats.LoadChat(ctx, chatID)
	if err != nil {
		return ChatDeletion{}, fmt.Errorf("agent chat folder: delete chat %s: %w", chatID, err)
	}
	if current.Type == domain.ChatTypeFolder {
		return ChatDeletion{}, fmt.Errorf("agent chat folder: %s is a folder: %w", chatID, apperr.ErrNotFound)
	}
	snapshot, err := u.workspaceSnapshotAround(ctx, current.WorkspaceID, current)
	if err != nil {
		return ChatDeletion{}, err
	}
	if wErr := guardNotWorking(snapshot.subtreeIDs(chatID), u.work); wErr != nil {
		return ChatDeletion{}, wErr
	}
	chats, folders := snapshot.subtree(chatID)
	chats = append(chats, chatID)
	orphaned, err := u.reapWorktrees(ctx, snapshot, chatID, chats, consent)
	if err != nil {
		return ChatDeletion{}, err
	}
	if err := u.purgeAll(ctx, snapshot, chats); err != nil {
		return ChatDeletion{}, err
	}
	u.remintOwners(ctx, orphaned)
	if err := u.removeAll(ctx, snapshot, folders); err != nil {
		return ChatDeletion{}, err
	}
	snapshot.plan.Reorder(snapshot.canonical(current.ParentID), "", -1)
	shifted, err := u.persist(ctx, snapshot)
	if err != nil {
		return ChatDeletion{}, err
	}
	return ChatDeletion{Chats: chats, Folders: folders, Shifted: shifted}, nil
}

// reapWorktrees tears down the worktree every chat in the doomed subtree owns,
// deepest first, so no reap runs against a workspace an earlier one's cascade
// already took. A workspace already gone (git-lineage cascade got there first)
// is tolerated, as is a chat with no workspace of its own (a bubble owns
// nothing). Each workspace id is reaped once regardless of how many doomed rows
// name it. Without consent, every workspace to reap is assessed first, so a
// refusal over work at risk leaves all of them in place.
//
// Returns the workspaces the subtree owned but could not reap because a
// surviving chat still holds them, so the caller can mint each a fresh owner
// (remintOwners) instead of leaving the next read to elect a survivor.
//
// rootID is the chat DeleteChat was actually asked to delete, as opposed to a
// descendant reached only via the cascade — the two must not answer a locked
// workspace the same way. A locked DESCENDANT is kept and reminted a fresh
// owner: the cascade caught it incidentally, and letting it survive is the
// whole point of "skip a locked child" (TestWorktree_deleteCascadeSkipsLockedChild).
// A locked ROOT is what the caller explicitly asked to delete — silently
// keeping ITS workspace and reporting 202 would tell the caller their delete
// succeeded when nothing about the thing they targeted was removed
// (TestRegression_DeleteLockedWorkspaceRejected expects 409, not a
// quietly-orphaned survivor).
func (u *chatFolderUsecase) reapWorktrees(
	ctx context.Context,
	snapshot *treeSnapshot,
	rootID string,
	ids []string,
	consent domain.DeleteConsent,
) ([]string, error) {
	reap, orphaned, err := u.planReap(ctx, snapshot, ids)
	if err != nil {
		return nil, err
	}
	if err := u.refuseLoss(ctx, reap, consent); err != nil {
		return nil, fmt.Errorf("agent chat folder: delete chat %s: %w", rootID, err)
	}
	for _, row := range reap {
		err = u.reaper.DeleteWorkspace(ctx, row.WorkspaceID, consent)
		if err == nil || errors.Is(err, apperr.ErrNotFound) {
			continue
		}
		// A locked DESCENDANT is kept, same as one a surviving chat still holds:
		// DeleteCascade correctly refuses a locked root, and that refusal means
		// this one worktree survives and needs a fresh owner. A locked ROOT is
		// what the caller asked to delete, so it falls through to the error.
		if errors.Is(err, workspace.ErrWorkspaceLocked) && row.ID != rootID {
			if u.ownsWorktree(ctx, snapshot, row) {
				orphaned = append(orphaned, row.WorkspaceID)
			}
			continue
		}
		return nil, fmt.Errorf("agent chat folder: delete chat %s: reap worktree %s: %w",
			row.ID, row.WorkspaceID, err)
	}
	return orphaned, nil
}

// planReap splits the doomed rows' workspaces into those to reap (one row per
// workspace) and those to keep: a worktree still held by a surviving chat
// (heldElsewhere), or the repo's main checkout, which only deleting the repo
// takes. A kept workspace the subtree owned is returned for a fresh owner.
func (u *chatFolderUsecase) planReap(
	ctx context.Context,
	snapshot *treeSnapshot,
	ids []string,
) ([]domain.Chat, []string, error) {
	doomed := doomedSet(ids)
	seen := make(map[string]bool, len(ids))
	var reap []domain.Chat
	var orphaned []string
	for _, id := range ids {
		row := snapshot.row(id)
		if row == nil || row.WorkspaceID == "" || seen[row.WorkspaceID] {
			continue
		}
		seen[row.WorkspaceID] = true
		shared, err := u.heldElsewhere(ctx, row.WorkspaceID, doomed)
		if err != nil {
			return nil, nil, fmt.Errorf("agent chat folder: delete chat %s: %w", id, err)
		}
		if !shared && !u.isDefaultCheckout(ctx, row.WorkspaceID) {
			reap = append(reap, *row)
			continue
		}
		if u.ownsWorktree(ctx, snapshot, *row) {
			orphaned = append(orphaned, row.WorkspaceID)
		}
	}
	return reap, orphaned, nil
}

// refuseLoss stops a delete without consent that would destroy work existing
// nowhere else in any workspace it reaps, naming that work.
func (u *chatFolderUsecase) refuseLoss(
	ctx context.Context,
	reap []domain.Chat,
	consent domain.DeleteConsent,
) error {
	if consent == domain.DiscardWorkAtRisk || len(reap) == 0 {
		return nil
	}
	wsIDs := make([]string, 0, len(reap))
	for _, row := range reap {
		wsIDs = append(wsIDs, row.WorkspaceID)
	}
	risks, err := u.reaper.WorkAtRisk(ctx, wsIDs)
	if err != nil {
		return err
	}
	if len(risks) > 0 {
		return &domain.WorkAtRiskError{Workspaces: risks}
	}
	return nil
}

// isDefaultCheckout reports whether wsID is its repo's main checkout.
func (u *chatFolderUsecase) isDefaultCheckout(
	ctx context.Context,
	wsID string,
) bool {
	repoID, err := u.workspaces.RepoOf(ctx, wsID)
	if err != nil || repoID == "" {
		return false
	}
	def, err := u.workspaces.DefaultWorkspaceOf(ctx, repoID)
	return err == nil && def == wsID
}

// ownsWorktree reports whether row is the chat that owns its workspace, by
// record or by the same resolution every wire surface makes.
func (u *chatFolderUsecase) ownsWorktree(
	ctx context.Context,
	snapshot *treeSnapshot,
	row domain.Chat,
) bool {
	if row.OwnsWorkspace {
		return true
	}
	var holders []domain.Chat
	for _, r := range snapshot.rows {
		if r.WorkspaceID == row.WorkspaceID {
			holders = append(holders, r)
		}
	}
	owner, ok := domain.ResolveOwningChat(holders)
	return ok && owner.ID == row.ID
}

// remintOwners gives each workspace a delete just left ownerless a fresh
// owning row, placed where its git parent's row is. Best-effort: the delete
// already happened, and the boot reconcile (reconcileOwningChats) mints any
// owner still missing.
func (u *chatFolderUsecase) remintOwners(
	ctx context.Context,
	workspaceIDs []string,
) {
	for _, wsID := range workspaceIDs {
		parent, _ := u.workspaces.VisibleForkParent(ctx, wsID)
		chatID, err := u.MintOwningChat(ctx, parent)
		if err == nil {
			err = u.AttachOwningWorkspace(ctx, chatID, domain.Workspace{ID: wsID})
		}
		if err != nil {
			slog.WarnContext(ctx, "agent chat folder: remint the owner of a shared worktree",
				"err", err, "workspace_id", wsID)
		}
	}
}

// purgeAll erases each chat in order and drops it from the plan as it goes, so
// the densify that follows counts only the rows that survived. A not-found
// from PurgeChat is tolerated: a row can be real at the tree level (parented,
// ordered, rendered) without ever having minted a conversation aggregate — a
// thread whose create never got past placement, say.
func (u *chatFolderUsecase) purgeAll(
	ctx context.Context,
	snapshot *treeSnapshot,
	ids []string,
) error {
	for _, id := range ids {
		err := u.agent.PurgeChat(ctx, id)
		if err != nil && !errors.Is(err, apperr.ErrNotFound) {
			return fmt.Errorf("agent chat folder: purge chat %s: %w", id, err)
		}
		snapshot.drop(id)
	}
	return nil
}

// removeAll erases each folder caught inside a purged subtree: a folder is a
// domain.Folder+domain.Node pair, never a PurgeChat-able row, so both halves
// are erased here, mirroring Delete's own folder erasure (tree.go).
func (u *chatFolderUsecase) removeAll(
	ctx context.Context,
	snapshot *treeSnapshot,
	ids []string,
) error {
	for _, id := range ids {
		if err := u.folders.Delete(ctx, id); err != nil {
			return fmt.Errorf("agent chat folder: delete %s: %w", id, err)
		}
		if err := u.nodes.Forget(ctx, id); err != nil {
			return fmt.Errorf("agent chat folder: delete %s: node: %w", id, err)
		}
		snapshot.drop(id)
	}
	return nil
}
