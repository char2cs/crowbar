package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The one boundary a WORKSPACE placement owes beyond the repo scope every row
// shares: its own fork chain. Organisation and git lineage are separate edges
// here — which is exactly why one can be dragged out from under the other.

// checkForkChainSplit refuses a WORKSPACE placement that would carry its row
// out of the space its own fork parent owns — the guard usecases/folder took
// with it when it was retired (11b72c720 flagged ErrForkChainSplit as having no
// unified-tree equivalent and left it out), restored here against the unified
// tree's own vocabulary.
//
// The invariant the model names explicitly: a forked workspace RENDERS under
// its fork parent, and domain.Workspace.ParentID — which a placement never
// rewrites (see its own doc) — is what merge eligibility, the diff base and the
// reparent leaf check each resolve back to a workspace. A row filed away from
// that parent contradicts all four: the sidebar drops the incompatible
// placement and draws the row under its fork parent anyway
// (buildSidebarTree's compatibleFolder), so the drag answers 200 and visibly
// does nothing. Reparenting is the explicit route for moving a fork's lineage,
// and the frontend already fires it FIRST for exactly these drops
// (drop-actions.ts's planTreeRowDrop, which waits for the reparent to land
// before sending the placement) — what this refuses is the raw placement that
// skips it.
//
// It runs only when the placement actually changes CONTAINERS, matching the
// retired guard's own shape (resolvePlacement returned early for a request that
// named no new folder). A reorder among the siblings a row already has splits
// nothing, and a row already sitting in a split position — persisted while no
// guard existed — has to stay draggable rather than wedge.
func (u *chatFolderUsecase) checkForkChainSplit(
	ctx context.Context,
	snapshot *treeSnapshot,
	move workspaceMove,
) error {
	if move.destination == move.origin {
		return nil
	}
	forkParentID, err := u.workspaces.VisibleForkParent(ctx, move.workspaceID)
	if err != nil {
		return fmt.Errorf("agent chat folder: place workspace %s: %w", move.workspaceID, err)
	}
	anchor, err := u.forkAnchorAt(ctx, snapshot, move.destination)
	if err != nil {
		return fmt.Errorf(
			"agent chat folder: place workspace %s under %s: %w",
			move.workspaceID, move.destination, err,
		)
	}
	if anchor == forkParentID {
		return nil
	}
	return fmt.Errorf(
		"agent chat folder: place workspace %s under %s, whose fork parent is %q, not %q: %w",
		move.workspaceID, move.destination, anchor, forkParentID, ErrForkChainSplit,
	)
}

// forkAnchorAt answers the workspace a row placed at id would fork FROM: the
// nearest ancestor-or-self carrying a WorkspaceID, and "" at the tree's root.
//
// It is CwdWorkspaceID's walk (walk.go) run over the same live reads every
// other guard here uses instead of a prebuilt forest, and deliberately the very
// same definition: that walk is what resolved a fork's ParentID when it was
// created (domain.Workspace.ParentID's own doc), so a placement is judged by
// the exact rule that minted the value it is judged against.
//
// Stopping at ANY row carrying a WorkspaceID is what separates it from
// nearestWorkspaceAnchor, which stops only at a Node{Kind:workspace} row. An
// ordinary fork draws no such row at all — its owning CHAT represents it 1:1
// (mergeHomeNode's own doc) — so a walk that stepped past one would report the
// locked branch further up as the anchor and refuse legal moves inside a fork's
// own subtree. A plain chat's ground workspace is the right answer here for the
// same reason: a fork filed under a chat is filed inside that chat's workspace.
func (u *chatFolderUsecase) forkAnchorAt(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (string, error) {
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		row, err := u.resolveRow(ctx, snapshot, id)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", id, err)
		}
		if row.WorkspaceID != "" {
			return row.WorkspaceID, nil
		}
		id = u.livePlacementOf(ctx, *row)
	}
	return "", nil
}

// livePlacementOf answers the container a row is in RIGHT NOW: its Node's
// ParentID for a Node-backed row, whose Chat.ParentID froze at creation
// (writeRow's dispatch, plan.go), and the row's own ParentID for one Node has
// never touched — the same precedence nearestWorkspaceAnchor's walk keeps.
func (u *chatFolderUsecase) livePlacementOf(
	ctx context.Context,
	row domain.Chat,
) string {
	n, err := u.nodes.GetNode(ctx, row.ID)
	if err != nil {
		return row.ParentID
	}
	return n.ParentID
}
