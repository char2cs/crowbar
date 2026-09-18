package tree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The rows a level draws that predate Node rows — a repo imported, a locked
// branch adopted or a chat filed before the Node aggregate existed — and the
// chats whose frozen parent names nothing any more. No backfill: each is
// counted where the sidebar draws it, and the first write that touches its
// level mints its Node at the decided slot.

// rerootDanglingChats files a chat whose frozen ParentID names nothing that
// exists any more (a Chats-panel folder from before Node rows, never
// migrated) at the root the sidebar draws it at, and answers the ids so the
// level's first write mints each one's Node at its decided slot.
func (u *chatFolderUsecase) rerootDanglingChats(
	ctx context.Context,
	f *forest,
	nodeSeen map[string]bool,
) []string {
	known := make(map[string]bool, len(f.rows))
	for _, row := range f.rows {
		known[row.ID] = true
	}
	var rerooted []string
	for i := range f.rows {
		row := &f.rows[i]
		if row.ParentID == "" || nodeSeen[row.ID] || known[row.ParentID] ||
			row.Type == domain.ChatTypeFolder || row.Type == nodePhantomType || row.Type == workspaceAnchorType {
			continue
		}
		if u.rowExists(ctx, row.ParentID) {
			continue
		}
		row.ParentID = ""
		rerooted = append(rerooted, row.ID)
	}
	return rerooted
}

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

// addNodelessAnchors appends a fresh anchor for every branch row of a repo
// scope that has no Node row yet (a locked branch from before Node rows
// existed): the sidebar draws it where its fork parent's row is, so the
// densify counts it there and the first write mints its Node.
func (u *chatFolderUsecase) addNodelessAnchors(
	ctx context.Context,
	f *forest,
	scope forestScope,
	nodeSeen map[string]bool,
) {
	if scope.repoID == "" {
		return
	}
	ids, err := u.workspaces.BranchRowsOf(ctx, scope.repoID)
	if err != nil {
		return
	}
	for _, id := range ids {
		if nodeSeen[id] || id == scope.workspaceID {
			continue
		}
		parent, _ := u.workspaces.VisibleForkParent(ctx, id)
		f.rows = append(f.rows, u.anchorView(ctx, id, domain.Node{ID: id, Kind: domain.NodeKindWorkspace, ParentID: parent}))
		f.homeIDs[id], f.fresh[id] = true, true
	}
}
