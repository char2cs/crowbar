package project

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// homeRow is one member of a project's home level as the repo placement pass
// sees it. fresh marks a home chat that has no Node row yet (created at the
// root before Node rows existed): it holds its frozen Chat.Order, and the
// first write that moves it mints its Node instead of ordering one.
type homeRow struct {
	domain.Node
	fresh     bool
	createdAt time.Time
}

// homeLevel answers ONE project's rows in the container folderID of its home
// level: the project's own repos, its home workspace's chats (Node-backed or
// not, never the hidden home owner) and its own home folders. Never a
// workspace anchor — a locked branch's Node shares the "" root but is drawn
// inside its repo — and never another project's rows. This is the member set
// the sidebar renders at the project top level, so an index computed against
// it lands where the drop line was drawn.
//
// homeWorkspaceID is "" when no home workspace resolves (or none is wired):
// chats then degrade to "every chat sharing the container", the posture
// every earlier version of this pass took.
func (u *projectUsecase) homeLevel(
	ctx context.Context,
	projectID string,
	folderID string,
	exclude string,
) (rows []homeRow, homeWorkspaceID string, err error) {
	repoIDs, err := u.repoIDSet(ctx, projectID)
	if err != nil {
		return nil, "", err
	}
	var chats []domain.Chat
	chatIDs := map[string]bool(nil)
	if u.workspaces != nil {
		if ws, hErr := u.workspaces.GetHomeForProject(ctx, projectID); hErr == nil {
			homeWorkspaceID = ws.ID
		}
	}
	if u.homeChats != nil && homeWorkspaceID != "" {
		chats, err = u.homeChats.ListByWorkspace(ctx, homeWorkspaceID)
		if err != nil {
			return nil, "", fmt.Errorf("project: reorder repos: list home chats: %w", err)
		}
		chats = withoutHomeOwner(chats)
		chatIDs = make(map[string]bool, len(chats))
		for _, c := range chats {
			chatIDs[c.ID] = true
		}
	}
	nodes, err := u.nodes.ListByParent(ctx, folderID)
	if err != nil {
		return nil, "", fmt.Errorf("project: reorder repos: list nodes: %w", err)
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
		return rows, homeWorkspaceID, nil
	}
	return u.withNodelessRows(ctx, rows, repoIDs, chats, seen, exclude), homeWorkspaceID, nil
}

// withNodelessRows appends the root rows that predate Node rows — a repo,
// or a home chat created before the root minted one — at the root the
// sidebar draws them at; their first move mints the row.
func (u *projectUsecase) withNodelessRows(
	ctx context.Context,
	rows []homeRow,
	repoIDs map[string]bool,
	chats []domain.Chat,
	seen map[string]bool,
	exclude string,
) []homeRow {
	for id := range repoIDs {
		if seen[id] || id == exclude {
			continue
		}
		rows = append(rows, homeRow{Node: domain.Node{ID: id, Kind: domain.NodeKindRepo}, fresh: true})
	}
	for _, c := range chats {
		if seen[c.ID] || c.ID == exclude || c.ParentID != "" {
			continue
		}
		if _, gErr := u.nodes.GetNode(ctx, c.ID); gErr == nil {
			continue // Node-backed, filed elsewhere
		}
		rows = append(rows, homeRow{
			Node:      domain.Node{ID: c.ID, Kind: domain.NodeKindChat, Order: c.Order},
			fresh:     true,
			createdAt: c.CreatedAt,
		})
	}
	return rows
}

// homeMember is homeLevel's per-kind membership rule for one Node row.
func (u *projectUsecase) homeMember(
	ctx context.Context,
	n domain.Node,
	repoIDs map[string]bool,
	chatIDs map[string]bool,
	homeWorkspaceID string,
) bool {
	switch n.Kind {
	case domain.NodeKindRepo:
		return repoIDs[n.ID]
	case domain.NodeKindChat:
		return chatIDs == nil || chatIDs[n.ID]
	case domain.NodeKindFolder:
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
	rows, _, err := u.homeLevel(ctx, projectID, "", "")
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
