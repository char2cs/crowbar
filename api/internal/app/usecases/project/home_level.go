package project

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// homeRow is one member of a project's home level. fresh marks a home chat predating Node rows: it holds
// its frozen Chat.Order, and the first write that moves it mints its Node.
type homeRow struct {
	tree.Node
	fresh     bool
	createdAt time.Time
}

// homeLevel answers one project's rows in container folderID of its home level — its repos, home chats
// (never the hidden owner) and home folders, never workspace anchors or other projects' rows — so an
// index lands where the sidebar drew the drop line. Without a home workspace, chats are every chat there.
func (u *projectUsecase) homeLevel(
	ctx context.Context,
	projectID string,
	folderID string,
	exclude string,
) (rows []homeRow, err error) {
	repoIDs, err := u.repoIDSet(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var chats []domain.Chat
	homeWorkspaceID := ""
	chatIDs := map[string]bool(nil)
	if u.workspaces != nil {
		if ws, hErr := u.workspaces.GetHomeForProject(ctx, projectID); hErr == nil {
			homeWorkspaceID = ws.ID
		}
	}
	if u.homeChats != nil && homeWorkspaceID != "" {
		chats, err = u.homeChats.ListByWorkspace(ctx, homeWorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("project: reorder repos: list home chats: %w", err)
		}
		chats = withoutHomeOwner(chats)
		chatIDs = make(map[string]bool, len(chats))
		for _, c := range chats {
			chatIDs[c.ID] = true
		}
	}
	nodes, err := u.nodes.ListByParent(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("project: reorder repos: list nodes: %w", err)
	}
	createdAt := make(map[string]time.Time, len(chats))
	for _, c := range chats {
		createdAt[c.ID] = c.CreatedAt
	}
	seen := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		seen[n.ID] = true
		if n.ID == exclude || !u.homeMember(ctx, n, repoIDs, chatIDs, homeWorkspaceID) {
			continue
		}
		rows = append(rows, homeRow{Node: n, createdAt: createdAt[n.ID]})
	}
	if folderID != "" {
		return rows, nil
	}
	return u.withNodelessRows(ctx, rows, repoIDs, chats, seen, exclude), nil
}

// withNodelessRows appends root rows predating Node rows (repos, old home chats); their first move mints
// the row. Absence from ListByParent("") is not "no Node" — a repo filed in a folder has one — so each
// miss is confirmed with GetNode before being claimed fresh.
func (u *projectUsecase) withNodelessRows(
	ctx context.Context,
	rows []homeRow,
	repoIDs map[string]bool,
	chats []domain.Chat,
	seen map[string]bool,
	exclude string,
) []homeRow {
	for _, c := range chats {
		if seen[c.ID] || c.ID == exclude {
			continue
		}
		if _, gErr := u.nodes.GetNode(ctx, c.ID); gErr == nil {
			continue // Node-backed, filed elsewhere
		}
		if c.ParentID != "" && u.homeRowExists(ctx, c.ParentID, chats) {
			continue // filed under a live row of the home
		}
		rows = append(rows, homeRow{
			Node:      tree.Node{ID: c.ID, Kind: tree.KindChat, Order: c.Order},
			fresh:     true,
			createdAt: c.CreatedAt,
		})
	}
	return rows
}

// homeRowExists reports whether id names a row a home chat can be filed
// under; a pre-Node Chats-panel folder id names nothing, and the sidebar
// draws such a chat at the root.
func (u *projectUsecase) homeRowExists(
	ctx context.Context,
	id string,
	chats []domain.Chat,
) bool {
	for _, c := range chats {
		if c.ID == id {
			return true
		}
	}
	if _, err := u.nodes.GetNode(ctx, id); err == nil {
		return true
	}
	if u.folders == nil {
		return false
	}
	f, err := u.folders.FindByKey(ctx, id)
	return err == nil && f != nil
}

// homeMember is homeLevel's per-kind membership rule for one Node row.
func (u *projectUsecase) homeMember(
	ctx context.Context,
	n tree.Node,
	repoIDs map[string]bool,
	chatIDs map[string]bool,
	homeWorkspaceID string,
) bool {
	switch n.Kind {
	case tree.KindRepo:
		return repoIDs[n.ID]
	case tree.KindChat:
		return chatIDs == nil || chatIDs[n.ID]
	case tree.KindFolder:
		if u.folders == nil {
			return true
		}
		f, err := u.folders.FindByKey(ctx, n.ID)
		return err == nil && f != nil && f.InHome(homeWorkspaceID)
	}
	return false
}

// withoutHomeOwner drops the chat that owns the home workspace itself: it is
// the workspace's own addressable row, not a row the top level draws.
func withoutHomeOwner(
	chats []domain.Chat,
) []domain.Chat {
	owner, ok := domain.ResolveOwningChat(chats)
	if !ok {
		return chats
	}
	out := make([]domain.Chat, 0, len(chats))
	for _, c := range chats {
		if c.ID != owner.ID {
			out = append(out, c)
		}
	}
	return out
}

// NextHomeSlot implements Usecase.
func (u *projectUsecase) NextHomeSlot(
	ctx context.Context,
	projectID string,
) (int, error) {
	rows, err := u.homeLevel(ctx, projectID, "", "")
	if err != nil {
		return 0, err
	}
	next := 0
	for _, row := range rows {
		if row.Order >= next {
			next = row.Order + 1
		}
	}
	return max(next, len(rows)), nil
}

// homeIndex is nodeIndex over homeLevel's rows.
func homeIndex(
	rows []homeRow,
) []slot {
	slots := make([]slot, 0, len(rows))
	for i, row := range rows {
		slots = append(slots, slot{at: i, id: row.ID, order: row.Order, rank: rankOfKind(row.Kind), createdAt: row.createdAt})
	}
	return slots
}
