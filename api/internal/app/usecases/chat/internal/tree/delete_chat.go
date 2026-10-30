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

// DeleteChat erases chatID's subtree and the worktrees it alone held. Every
// refusal comes before the first effect, since a reap cannot be undone; reaps
// precede purges because a chat is the only handle on its worktree.
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
	plan.levels = plan.rehang(snapshot)
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
	for _, level := range append(plan.levels, snapshot.canonical(current.ParentID)) {
		snapshot.plan.Reorder(level, "", -1)
	}
	shifted, err := u.persist(ctx, snapshot)
	if err != nil {
		return ChatDeletion{}, err
	}
	return ChatDeletion{Chats: plan.chats, Folders: plan.folders, Shifted: shifted, Taken: plan.taken}, nil
}

// deletion is everything one DeleteChat takes, settled before any of it goes.
// taken is the rows outside the selection whose worktree the cascade takes;
// levels the sibling spaces the rows they held rise into.
type deletion struct {
	chats    []string
	folders  []string
	taken    []string
	levels   []string
	reap     []domain.Chat
	orphaned []string
}

// planDeletion settles what the delete takes and answers every refusal before
// an effect. The reap is bounded by git lineage: a row outside the selection
// goes only when the cascade takes its own worktree.
func (u *chatFolderUsecase) planDeletion(
	ctx context.Context,
	snapshot *treeSnapshot,
	rootID string,
	consent domain.DeleteConsent,
) (deletion, error) {
	chats, folders := snapshot.subtree(rootID)
	d := deletion{chats: append(chats, rootID), folders: folders}
	reap, orphaned, err := u.planReap(ctx, snapshot, d.chats)
	if err != nil {
		return deletion{}, err
	}
	reap, orphaned, cascaded, err := u.planCascades(ctx, snapshot, rootID, reap, orphaned)
	if err != nil {
		return deletion{}, fmt.Errorf("agent chat folder: delete chat %s: %w", rootID, err)
	}
	d.reap, d.orphaned = reap, orphaned
	d.takeRowsOf(snapshot, cascaded)
	if err := guardNotWorking(ctx, d.taken, u.busy); err != nil {
		return deletion{}, err
	}
	if err := u.refuseLoss(ctx, d.reap, consent); err != nil {
		return deletion{}, fmt.Errorf("agent chat folder: delete chat %s: %w", rootID, err)
	}
	return d, nil
}

// planCascades asks the workspace layer what each reap would take. A locked
// descendant the cascade only caught is kept and reminted an owner; a locked
// root is what the caller named, so it refuses rather than remove nothing.
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

// takeRowsOf adds the rows outside the selection anchored in a worktree the
// cascade takes — each row alone; what it holds is re-hung, not cascaded.
func (d *deletion) takeRowsOf(
	snapshot *treeSnapshot,
	cascaded map[string]bool,
) {
	doomed := doomedSet(append(append([]string{}, d.chats...), d.folders...))
	for _, row := range snapshot.rows {
		if cascaded[row.WorkspaceID] && !doomed[row.ID] {
			d.taken = append(d.taken, row.ID)
		}
	}
	d.chats = append(d.chats, d.taken...)
}

// rehang lifts what each taken row holds outside the delete to that row's own
// parent, as deleting a folder promotes what it held, and returns those levels.
func (d *deletion) rehang(snapshot *treeSnapshot) []string {
	doomed := doomedSet(append(append([]string{}, d.chats...), d.folders...))
	var levels []string
	for _, id := range d.taken {
		node, ok := snapshot.plan.Node(id)
		if !ok {
			continue
		}
		for _, child := range snapshot.plan.Members(snapshot.container(id)) {
			if !doomed[child.ID] {
				snapshot.plan.SetParent(child.ID, node.ParentID)
			}
		}
		levels = append(levels, node.ParentID)
	}
	return levels
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

// planReap picks one row per doomed workspace to reap, keeping a worktree a
// surviving chat still holds or one that outlives its chats (the repo's main
// checkout); a kept one the subtree owned is returned for a fresh owner.
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
