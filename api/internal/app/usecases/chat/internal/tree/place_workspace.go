package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// PlaceWorkspace moves a workspace's own row within its repo's tree — the
// row workspaceAnchorView renders for its Node{Kind:workspace} row (2026-09-09
// sidebar-placement-unification, workspace-placement fix). It shares
// checkWorkspaceMove/checkFolderContainer's repo-scoped golden rule rather
// than checkChatMove's same-workspace one — see checkWorkspaceMove's own doc
// for why.
//
// Every worktree-owning workspace goes through this call, LOCKED branch or
// ordinary fork alike — not gated on RendersAsBranch. That predicate answers
// a different, narrower question for a different caller: whether
// mergeHomeNode's own shared BFS walk should discover this workspace's Node
// row ALONGSIDE its owning chat's (true only for a locked branch, whose
// owning chat is placed through the pre-Node placeOwningRow path and so
// never grows a Node{Kind:chat} row of its own to collide with — see
// mergeHomeNode's own doc). PlaceWorkspace never depends on that walk for
// its OWN subject: it resolves workspaceID directly (RepoOf, Nodes.GetNode),
// so gating it the same way would refuse the one call the frontend already
// fires for EVERY worktree-owning row's drag (drop-actions.ts's
// planTreeRowDrop, `kind: 'branch'`, unconditional on lock status) — a
// regression caught before shipping: an ordinary fork's reorder used to be a
// silent no-op through the old chat-addressed route (nothing ever rendered
// its Chat.Order — see dto.ChatWorktreeDTO's own gap, closed alongside this
// fix), and gating this call the same way RendersAsBranch is gated would
// have turned that silent no-op into a loud 404 instead of fixing it.
//
// It refuses with apperr.ErrNotFound only for a workspaceID RepoOf cannot
// resolve to a real repo — a nonexistent id, or the project's own HOME
// workspace (RepoOf answers "" for it; it is never a member of a repo's
// tree at all, and the frontend already never calls this for it — see
// planTreeRowDrop's own defaultWorkspaceId guard).
//
// A workspace whose Node row has never been touched (a pre-existing
// workspace older than the Node migration itself, or simply one whose
// position this call is the very first write to — the identical gap
// project.go's repo fix closed, see ensureSubjectWritten) degrades to a
// zero-value ParentID/Order exactly like a fresh mint: globalSnapshotAround's
// subjectIsNodeBacked marks it snapshot.freshIDs when mergeForest's own walk
// never discovered a Node row for it either, and persist's dispatch
// (writeHomeNode) mints one via Nodes.Create rather than trying to move a
// row that was never there.
func (u *chatFolderUsecase) PlaceWorkspace(
	ctx context.Context,
	workspaceID string,
	in PlaceInput,
) (domain.Chat, []domain.Chat, error) {
	repoID, err := u.workspaces.RepoOf(ctx, workspaceID)
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: place workspace %s: %w", workspaceID, err)
	}
	if repoID == "" {
		return domain.Chat{}, nil, fmt.Errorf(
			"agent chat folder: place workspace %s: %w", workspaceID, apperr.ErrNotFound)
	}
	n, nErr := u.nodes.GetNode(ctx, workspaceID)
	if nErr != nil {
		n = domain.Node{}
	}
	current := workspaceAnchorView(workspaceID, n)
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if mErr := u.checkWorkspaceMove(ctx, snapshot, repoID, workspaceID, destination); mErr != nil {
		return domain.Chat{}, nil, mErr
	}
	if wErr := guardNotWorking(subtreeIDsOf(workspaceID, snapshot.rows), u.work); wErr != nil {
		return domain.Chat{}, nil, wErr
	}
	u.replace(snapshot, workspaceID, current.ParentID, destination, in.Order)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	return *snapshot.placedRow(workspaceID), without(written, workspaceID), nil
}
