package tree

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// Read once, plan the whole change in memory, write only the rows that moved: the chat read model is an
// async projection, so a re-read between a write and the renumber after it can serve the pre-write list.

// loadChat resolves a chat from the log fold, with its live Node placement, as the origin a move is planned
// against; a stale origin would file the row back under its previous parent. A chat of another workspace
// is NOT-FOUND, so the caller learns nothing about rows outside its scope.
func (u *chatFolderUsecase) loadChat(
	ctx context.Context,
	workspaceID string,
	chatID string,
) (domain.Chat, error) {
	chat, err := u.chats.LoadChat(ctx, chatID)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: get chat %s: %w", chatID, err)
	}
	if chat.WorkspaceID != workspaceID {
		return domain.Chat{}, fmt.Errorf(
			"agent chat folder: chat %s is not in workspace %s: %w", chatID, workspaceID, problem.ErrNotFound,
		)
	}
	return u.correctHomePlacement(ctx, chat)
}

// globalSnapshotAround reads every row the daemon knows, merged with the Node forest in subject's scope,
// with subject's row taken from the caller's log fold (see corrected). Folders, workspace anchors and an
// ordinary fork's owning chat (ChatTypeBranch) are Node-backed subjects; their writes go to Nodes.
func (u *chatFolderUsecase) globalSnapshotAround(
	ctx context.Context,
	subject domain.Chat,
) (*treeSnapshot, error) {
	scope, err := u.scopeForSubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	return u.globalSnapshotIn(ctx, subject, scope)
}

// globalSnapshotIn is globalSnapshotAround with the root's scope already
// resolved — for a folder create, whose subject does not exist yet.
func (u *chatFolderUsecase) globalSnapshotIn(
	ctx context.Context,
	subject domain.Chat,
	scope forestScope,
) (*treeSnapshot, error) {
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: %w", err)
	}
	f, err := u.mergeForest(ctx, rows, scope, []string{subject.ParentID})
	if err != nil {
		return nil, err
	}
	subjectIsNodeBacked := subject.ID != "" &&
		(subject.Type == domain.ChatTypeFolder || subject.Type == domain.ChatTypeBranch ||
			subject.Type == workspaceAnchorType)
	return buildHomeSnapshot(f, subject, subjectIsNodeBacked), nil
}

// workspaceSnapshot is the snapshot chat placement plans against for workspaceID's level, folders and
// every other kind interleaved, as of one read.
func (u *chatFolderUsecase) workspaceSnapshot(
	ctx context.Context,
	workspaceID string,
) (*treeSnapshot, error) {
	return u.workspaceSnapshotAround(ctx, workspaceID, domain.Chat{})
}

// workspaceSnapshotAround is workspaceSnapshot with subject's row taken from the caller's log fold: a stale
// container would turn a reorder into a move or drop a move, and a just-minted subject is appended.
// seeds are containers the walk must reach (the destination), so a first placement lands last, not at 0.
func (u *chatFolderUsecase) workspaceSnapshotAround(
	ctx context.Context,
	workspaceID string,
	subject domain.Chat,
	seeds ...string,
) (*treeSnapshot, error) {
	// Every row, not one workspace's: a level spans workspaces (a repo root holds the default checkout's
	// chats and every fork's own row), and the scope decides who counts.
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: chats: %w", err)
	}
	if workspaceID == "" {
		f, err := u.mergeForest(ctx, rows, u.scopeForBubble(ctx, subject), seeds)
		if err != nil {
			return nil, err
		}
		delete(f.foreign, subject.ID)
		snap := newTreeSnapshotScoped(corrected(f.rows, subject), f.foreign, f.aliases)
		snap.homeIDs, snap.freshIDs = f.homeIDs, f.fresh
		return snap, nil
	}
	return u.homeSnapshotAround(ctx, workspaceID, rows, subject, seeds...)
}

// corrected replaces the projected row for subject with the log-folded one, or
// adds it when the projection has not caught up enough to list it at all.
func corrected(
	rows []domain.Chat,
	subject domain.Chat,
) []domain.Chat {
	if subject.ID == "" {
		return rows
	}
	for i := range rows {
		if rows[i].ID == subject.ID {
			rows[i] = subject
			return rows
		}
	}
	return append(rows, subject)
}

// persist writes the rows the plan touched plus every fresh row (owed a Node Create even when its placement
// equals the zero value), and returns the folders and anchors (as branch rows) for the caller to broadcast,
// since no projection announces them; a chat's write is broadcast by the hub projection.
func (u *chatFolderUsecase) persist(
	ctx context.Context,
	snapshot *treeSnapshot,
) ([]domain.Chat, error) {
	ids := snapshot.plan.Dirty()
	seen := make(map[string]bool, len(ids)+len(snapshot.freshIDs))
	written := make([]domain.Chat, 0, len(ids))
	writeOne := func(id string) error {
		seen[id] = true
		row, err := u.writeRow(ctx, snapshot, id)
		if err != nil {
			return err
		}
		switch {
		case row == nil:
		case row.Type == domain.ChatTypeFolder:
			written = append(written, *row)
		case row.Type == workspaceAnchorType:
			anchor := *row
			anchor.Type = domain.ChatTypeBranch
			written = append(written, anchor)
		}
		return nil
	}
	for _, id := range ids {
		if err := writeOne(id); err != nil {
			return nil, err
		}
	}
	for id := range snapshot.freshIDs {
		if seen[id] {
			continue
		}
		if err := writeOne(id); err != nil {
			return nil, err
		}
	}
	return written, nil
}

// writeRow saves one row: a re-parented row has its whole placement written, a renumbered one only its
// order, so a densify never restates a parent read from a possibly stale projection. Node-backed rows
// (snapshot.homeIDs) go to writeHomeNode.
func (u *chatFolderUsecase) writeRow(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (*domain.Chat, error) {
	row := snapshot.placedRow(id)
	if row == nil {
		return nil, nil
	}
	if snapshot.homeIDs[id] {
		return u.writeHomeNode(ctx, snapshot, row)
	}
	if !snapshot.reparented(id) {
		updated, err := u.chats.SetOrder(ctx, id, row.Order)
		if err != nil {
			// A concurrent delete can purge the row between snapshot and write; a row
			// that no longer exists needs no order.
			if errors.Is(err, problem.ErrNotFound) {
				return nil, nil
			}
			return nil, fmt.Errorf("agent chat folder: order chat %s: %w", id, err)
		}
		return &updated, nil
	}
	updated, err := u.chats.SetPlacement(ctx, id, row.ParentID, row.Order)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: place %s: %w", id, err)
	}
	return &updated, nil
}

// without drops the subject row from a written set, leaving the collateral the
// caller broadcasts alongside it.
func without(
	rows []domain.Chat,
	id string,
) []domain.Chat {
	out := make([]domain.Chat, 0, len(rows))
	for _, row := range rows {
		if row.ID != id {
			out = append(out, row)
		}
	}
	return out
}

// placementTarget resolves the index a moved row should land at: the caller's
// explicit request, its current index when it is only being re-parented in
// place, or the end of the destination when it arrives from elsewhere.
func placementTarget(
	requested *int,
	snapshot *treeSnapshot,
	origin string,
	destination string,
	id string,
	firstPlacement bool,
) int {
	if requested != nil {
		return *requested
	}
	if snapshot.canonical(origin) == snapshot.canonical(destination) && !firstPlacement {
		return snapshot.plan.IndexOf(snapshot.canonical(destination), id)
	}
	return snapshot.plan.NextSlot(snapshot.canonical(destination))
}

func cleanName(
	name string,
) (string, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "", ErrNameRequired
	}
	return clean, nil
}
