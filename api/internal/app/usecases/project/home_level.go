package project

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type homeRow struct {
	tree.Node
	fresh     bool
	createdAt time.Time
}

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
			continue
		}
		if c.ParentID != "" && u.homeRowExists(ctx, c.ParentID, chats) {
			continue
		}
		rows = append(rows, homeRow{
			Node:      tree.Node{ID: c.ID, Kind: tree.KindChat, Order: c.Order},
			fresh:     true,
			createdAt: c.CreatedAt,
		})
	}
	return rows
}

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

func homeIndex(
	rows []homeRow,
) []slot {
	slots := make([]slot, 0, len(rows))
	for i, row := range rows {
		slots = append(slots, slot{at: i, id: row.ID, order: row.Order, rank: rankOfKind(row.Kind), createdAt: row.createdAt})
	}
	return slots
}

func (u *projectUsecase) placeRepoAmongHomeSiblings(
	ctx context.Context,
	projectID string,
	folderID string,
	subject tree.Node,
	target int,
) ([]tree.Node, error) {
	siblings, err := u.homeLevel(ctx, projectID, folderID, subject.ID)
	if err != nil {
		return nil, err
	}
	rows := append(siblings, homeRow{Node: subject})
	slots := homeIndex(rows)
	reparented := subject.ParentID != folderID
	decided, written, err := u.writeHomeLevel(ctx, rows, place(slots, subject.ID, &target),
		folderID, subject.ID, reparented)
	if err != nil {
		return nil, err
	}
	n, err := u.ensureSubjectWritten(ctx, slots, subject.ID, reparented, folderID, &target, written)
	if err != nil {
		return nil, err
	}
	if n != nil {
		decided = append(decided, *n)
	}
	return decided, nil
}

func (u *projectUsecase) densifyHomeLevel(
	ctx context.Context,
	projectID string,
	folderID string,
	exclude string,
) ([]tree.Node, error) {
	rows, err := u.homeLevel(ctx, projectID, folderID, exclude)
	if err != nil {
		return nil, err
	}
	decided, _, err := u.writeHomeLevel(ctx, rows, place(homeIndex(rows), "", nil),
		folderID, "", false)
	return decided, err
}

func (u *projectUsecase) writeHomeLevel(
	ctx context.Context,
	rows []homeRow,
	moves []move,
	folderID string,
	subjectID string,
	reparented bool,
) ([]tree.Node, map[string]bool, error) {
	written := make(map[string]bool, len(rows))
	var decided []tree.Node
	for _, moved := range moves {
		row := rows[moved.at]
		written[row.ID] = true
		decided = append(decided, tree.Node{ID: row.ID, Kind: row.Kind, ParentID: folderID, Order: moved.order})
		if err := u.writeHomeRow(ctx, row, folderID, moved.order, row.ID == subjectID && reparented); err != nil {
			return nil, nil, err
		}
	}
	for _, row := range rows {
		if !row.fresh || written[row.ID] || row.ID == subjectID {
			continue
		}
		written[row.ID] = true
		if err := u.mintNode(ctx, row.ID, row.Kind, folderID, row.Order); err != nil {
			return nil, nil, err
		}
	}
	return decided, written, nil
}

func (u *projectUsecase) writeHomeRow(
	ctx context.Context,
	row homeRow,
	folderID string,
	order int,
	reparenting bool,
) error {
	if row.fresh {
		return u.mintNode(ctx, row.ID, row.Kind, folderID, order)
	}
	return u.writeNode(ctx, row.ID, folderID, order, reparenting)
}
