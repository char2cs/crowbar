package tree

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// The cascade that takes a chat's threads and worktrees with it.

// DeleteChat erases chatID and every chat threaded below it, together with any
// worktree that subtree was the last thing holding. A folder id is refused as
// not-found — the folder verb promotes what it held, this one cascades.
//
// Every refusal is answered before the first effect (planDeletion): a reap
// cannot be undone, so a delete that will be refused must not have started.
// Worktrees are reaped before any chat is purged: a chat is the only handle on
// its worktree, so a failed reap fails the delete rather than orphan it.
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
		return ChatDeletion{}, fmt.Errorf("agent chat folder: %s is a folder: %w", chatID, problem.ErrNotFound)
	}
	snapshot, err := u.workspaceSnapshotAround(ctx, current.WorkspaceID, current)
	if err != nil {
		return ChatDeletion{}, err
	}
	if wErr := guardNotWorking(ctx, snapshot.subtreeIDs(chatID), u.busy); wErr != nil {
		return ChatDeletion{}, wErr
	}
	plan, err := u.planDeletion(ctx, snapshot, chatID, consent)
	if err != nil {
		return ChatDeletion{}, err
	}
	if err := u.reapWorktrees(ctx, chatID, plan.reap, consent); err != nil {
		return ChatDeletion{}, err
	}
	if err := u.purgeAll(ctx, snapshot, plan.chats); err != nil {
		return ChatDeletion{}, err
	}
	u.remintOwners(ctx, plan.orphaned)
	if err := u.removeAll(ctx, snapshot, plan.folders); err != nil {
		return ChatDeletion{}, err
	}
	snapshot.plan.Reorder(snapshot.canonical(current.ParentID), "", -1)
	shifted, err := u.persist(ctx, snapshot)
	if err != nil {
		return ChatDeletion{}, err
	}
	return ChatDeletion{Chats: plan.chats, Folders: plan.folders, Shifted: shifted}, nil
}

// deletion is everything one DeleteChat takes, settled before any of it goes.
type deletion struct {
	chats    []string
	folders  []string
	reap     []domain.Chat
	orphaned []string
}

// planDeletion settles what the delete takes and answers every refusal without
// an effect. The git cascade of a reaped worktree can take worktrees whose rows
// sit outside the subtree (a fork child dragged beside its parent); their rows
// join the doomed set, or the tree write would name rows the cascade erased.
func (u *chatFolderUsecase) planDeletion(
	ctx context.Context,
	snapshot *treeSnapshot,
	rootID string,
	consent domain.DeleteConsent,
) (deletion, error) {
	chats, folders := snapshot.subtree(rootID)
	d := deletion{chats: append(chats, rootID), folders: folders}
	for {
		reap, orphaned, err := u.planReap(ctx, snapshot, d.chats)
		if err != nil {
			return deletion{}, err
		}
		reap, orphaned, taken, err := u.planCascades(ctx, snapshot, rootID, reap, orphaned)
		if err != nil {
			return deletion{}, fmt.Errorf("agent chat folder: delete chat %s: %w", rootID, err)
		}
		d.reap, d.orphaned = reap, orphaned
		if !d.takeRowsOf(snapshot, taken) {
			break
		}
	}
	// Rows the cascade added are guarded as the subtree was.
	if err := guardNotWorking(ctx, d.chats, u.busy); err != nil {
		return deletion{}, err
	}
	if err := u.refuseLoss(ctx, d.reap, consent); err != nil {
		return deletion{}, fmt.Errorf("agent chat folder: delete chat %s: %w", rootID, err)
	}
	return d, nil
}

// planCascades asks the workspace layer what each reap would take. A locked
// DESCENDANT is kept and reminted a fresh owner — the cascade caught it
// incidentally (TestWorktree_deleteCascadeSkipsLockedChild); a locked ROOT is
// what the caller asked to delete, so it refuses (409) rather than report a
// delete that removed nothing (TestRegression_DeleteLockedWorkspaceRejected).
func (u *chatFolderUsecase) planCascades(
	ctx context.Context,
	snapshot *treeSnapshot,
	rootID string,
	planned []domain.Chat,
	orphaned []string,
) ([]domain.Chat, []string, map[string]bool, error) {
	taken := map[string]bool{}
	reap := make([]domain.Chat, 0, len(planned))
	for _, row := range planned {
		ids, err := u.reaper.DeletionPlan(ctx, row.WorkspaceID)
		switch {
		case err == nil:
			reap = append(reap, row)
			for _, id := range ids {
				taken[id] = true
			}
		case errors.Is(err, problem.ErrNotFound):
		case errors.Is(err, workspace.ErrWorkspaceLocked) && row.ID != rootID:
			if u.ownsWorktree(ctx, snapshot, row) {
				orphaned = append(orphaned, row.WorkspaceID)
			}
		default:
			return nil, nil, nil, fmt.Errorf("reap worktree %s: %w", row.WorkspaceID, err)
		}
	}
	return reap, orphaned, taken, nil
}

// takeRowsOf adds every row anchored in a taken workspace, with its subtree,
// to the doomed set, reporting whether that grew it.
func (d *deletion) takeRowsOf(
	snapshot *treeSnapshot,
	taken map[string]bool,
) bool {
	doomed := doomedSet(append(append([]string{}, d.chats...), d.folders...))
	grew := false
	for _, row := range snapshot.rows {
		if !taken[row.WorkspaceID] || doomed[row.ID] {
			continue
		}
		chats, folders := snapshot.appendSubtree(row.ID, nil, nil)
		for _, id := range append(chats, folders...) {
			doomed[id] = true
		}
		d.chats, d.folders = append(d.chats, chats...), append(d.folders, folders...)
		grew = true
	}
	return grew
}

// reapWorktrees tears down the planned worktrees, deepest first. A workspace an
// earlier reap's git-lineage cascade already took answers not-found, which is
// the outcome asked for.
func (u *chatFolderUsecase) reapWorktrees(
	ctx context.Context,
	rootID string,
	reap []domain.Chat,
	consent domain.DeleteConsent,
) error {
	for _, row := range reap {
		err := u.reaper.DeleteWorkspace(ctx, row.WorkspaceID, consent)
		if err != nil && !errors.Is(err, problem.ErrNotFound) {
			return fmt.Errorf("agent chat folder: delete chat %s: reap worktree %s: %w",
				rootID, row.WorkspaceID, err)
		}
	}
	return nil
}

// planReap splits the doomed rows' workspaces into those to reap (one row per
// workspace) and those to keep: a worktree still held by a surviving chat
// (heldElsewhere), or the repo's main checkout, which only deleting the repo
// takes. A kept workspace the subtree owned is returned for a fresh owner. A
// row with no workspace of its own (a bubble) owns nothing to reap.
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
		if !shared && !u.outlivesItsChats(ctx, row.WorkspaceID) {
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
		return domain.WorkAtRisked(risks)
	}
	return nil
}

// outlivesItsChats asks the workspace layer, which owns the answer, whether
// wsID survives the chats in it.
func (u *chatFolderUsecase) outlivesItsChats(ctx context.Context, wsID string) bool {
	outlives, err := u.workspaces.OutlivesItsChats(ctx, wsID)
	return err == nil && outlives
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
		parent, err := u.workspaces.VisibleForkParent(ctx, wsID)
		var chatID string
		if err == nil {
			chatID, err = u.MintOwningChat(ctx, "", parent)
		}
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
		if err != nil && !errors.Is(err, problem.ErrNotFound) {
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
