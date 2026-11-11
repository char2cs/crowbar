package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Rows a level draws that predate Node rows (an old repo import or locked branch). No backfill: each is
// counted where the sidebar draws it, and the first write touching its level mints its Node.

// rowExists reports whether id names any row a chat can be filed under.
func (u *chatFolderUsecase) rowExists(
	ctx context.Context,
	id string,
) bool {
	if _, err := u.nodes.GetNode(ctx, id); err == nil {
		return true
	}
	if _, err := u.chats.Get(ctx, id); err == nil {
		return true
	}
	if f, err := u.folders.FindByKey(ctx, id); err == nil && f != nil {
		return true
	}
	live, err := u.workspaces.Exists(ctx, id)
	return err == nil && live
}

// addNodelessRepos appends a fresh phantom for every repo of a home scope
// that has no Node row yet.
func (f *forest) addNodelessRepos(
	scope forestScope,
	nodeSeen map[string]bool,
) {
	if !scope.home {
		return
	}
	for id := range scope.repoMemberIDs {
		if nodeSeen[id] {
			continue
		}
		f.rows = append(f.rows, domain.Chat{ID: id, Type: nodePhantomType})
		f.homeIDs[id], f.fresh[id] = true, true
	}
}

// addNodelessAnchors appends a fresh anchor for every branch row of a repo scope with no Node row yet,
// placed under its fork parent's row where the sidebar draws it, so the densify counts it there.
func (u *chatFolderUsecase) addNodelessAnchors(
	ctx context.Context,
	f *forest,
	scope forestScope,
	nodeSeen map[string]bool,
) error {
	if scope.repoID == "" {
		return nil
	}
	ids, err := u.workspaces.BranchRowsOf(ctx, scope.repoID)
	if err != nil {
		return fmt.Errorf("agent chat folder: branch rows of %s: %w", scope.repoID, err)
	}
	for _, id := range ids {
		if nodeSeen[id] || id == scope.workspaceID {
			continue
		}
		parent, err := u.workspaces.VisibleForkParent(ctx, id)
		if err != nil && problem.CodeOf(err) != problem.NotFound {
			return fmt.Errorf("agent chat folder: fork parent of %s: %w", id, err)
		}
		anchor, err := u.anchorView(ctx, id, dtree.Node{ID: id, Kind: dtree.KindWorkspace, ParentID: parent})
		if err != nil {
			return err
		}
		f.rows = append(f.rows, anchor)
		f.homeIDs[id], f.fresh[id] = true, true
	}
	return nil
}
