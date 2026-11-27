package tree

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

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

func (u *chatFolderUsecase) workspaceSnapshot(
	ctx context.Context,
	workspaceID string,
) (*treeSnapshot, error) {
	return u.workspaceSnapshotAround(ctx, workspaceID, domain.Chat{})
}

func (u *chatFolderUsecase) workspaceSnapshotAround(
	ctx context.Context,
	workspaceID string,
	subject domain.Chat,
	seeds ...string,
) (*treeSnapshot, error) {
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
