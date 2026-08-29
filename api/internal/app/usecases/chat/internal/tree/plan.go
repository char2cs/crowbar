package tree

import (
	"context"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Read once, plan the whole change in memory, write only the rows that moved.
//
// That is not an optimisation. The chat read model is an asynchronous projection,
// so a re-read taken between a write and the renumber that follows it can still be
// serving the pre-write list. Planning from a single snapshot removes the race
// rather than papering over it with a barrier.

// load resolves a FOLDER row by id, log-folded so it is never stale. A row this
// call names that turns out to be a CHAT is refused as not-found: from this
// API's own vocabulary that id simply does not name a folder, and any other
// answer would let a rename or a delete reach a conversation through the wrong
// door.
func (u *chatFolderUsecase) load(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	row, err := u.chats.LoadChat(ctx, id)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: get %s: %w", id, err)
	}
	if row.Type != domain.ChatTypeFolder {
		return domain.Chat{}, fmt.Errorf("agent chat folder: %s: %w", id, apperr.ErrNotFound)
	}
	return row, nil
}

// loadChat resolves a chat and refuses one anchored to another workspace. It is
// a NOT-FOUND rather than a cross-workspace refusal: the caller addressed a row
// that does not exist in the scope it asked in, and answering otherwise would
// tell it that a chat it may not touch exists.
//
// It reads the LOG FOLD: the ParentID it hands back is the origin the move is
// planned against — the level to close up behind the row, and, for a reorder
// naming no destination, the level being reordered within. A stale origin does
// not merely misnumber the row, it files it back under the parent it had before
// the previous write moved it.
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
			"agent chat folder: chat %s is not in workspace %s: %w", chatID, workspaceID, apperr.ErrNotFound,
		)
	}
	return chat, nil
}

// globalSnapshot reads every row the daemon knows — the whole forest folder CRUD
// plans against, since a folder carries no workspace of its own to scope a
// narrower read by.
func (u *chatFolderUsecase) globalSnapshot(
	ctx context.Context,
) (*treeSnapshot, error) {
	return u.globalSnapshotAround(ctx, domain.Chat{})
}

// globalSnapshotAround is globalSnapshot with the SUBJECT's row corrected from
// the log fold the caller already holds — see corrected.
func (u *chatFolderUsecase) globalSnapshotAround(
	ctx context.Context,
	subject domain.Chat,
) (*treeSnapshot, error) {
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: %w", err)
	}
	return newTreeSnapshot(corrected(rows, subject)), nil
}

// workspaceSnapshot reads one workspace's rows, PLUS every folder, as of a
// single moment — the read chat placement plans against.
//
// Folders are read unscoped because they carry no workspace of their own (see
// the package doc): the two kinds still share one sibling space in the panel,
// so a chat's densify must see the folders interleaved with it or a folder
// sitting in that same level goes unrenumbered. Reading every folder in the
// daemon to renumber one workspace's level is over-broad — the real boundary
// is stage 3's walk — but the alternative, missing a folder the densify must
// touch, corrupts the very order this snapshot exists to keep dense.
func (u *chatFolderUsecase) workspaceSnapshot(
	ctx context.Context,
	workspaceID string,
) (*treeSnapshot, error) {
	return u.workspaceSnapshotAround(ctx, workspaceID, domain.Chat{})
}

// workspaceSnapshotAround is the same read with the SUBJECT's row corrected from
// the log fold the caller already holds.
//
// The rest of the list is read to renumber levels, and the projection is right
// enough for that. The subject is different in kind: the plan compares its
// stored container against the destination to decide whether this is a move at
// all, and either stale answer is damaging — an OLD container turns a reorder
// into a move back to it, and one that already shows the destination turns a
// move into a renumber and drops the write.
//
// A subject the list does not carry is appended, not skipped: a chat is minted
// and placed in one breath, long before the projection lists it, and a plan
// without it would discard the very placement that makes it a thread.
func (u *chatFolderUsecase) workspaceSnapshotAround(
	ctx context.Context,
	workspaceID string,
	subject domain.Chat,
) (*treeSnapshot, error) {
	rows, err := u.chats.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: chats: %w", err)
	}
	all, err := u.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: folders: %w", err)
	}
	for _, row := range all {
		if row.Type == domain.ChatTypeFolder {
			rows = append(rows, row)
		}
	}
	return newTreeSnapshot(corrected(rows, subject)), nil
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

// persist writes exactly the rows the plan touched and returns the FOLDER rows
// among them, which the caller has to broadcast itself. The CHAT rows still
// need no such handling: their write is an aggregate command, so the hub
// projection broadcasts each one on the way through — true of a folder row's
// write now too, but the wire contract this feeds is a folder-only list, so a
// densified chat sibling stays reported through its own channel instead of
// this one.
func (u *chatFolderUsecase) persist(
	ctx context.Context,
	snapshot *treeSnapshot,
) ([]domain.Chat, error) {
	ids := snapshot.plan.Dirty()
	written := make([]domain.Chat, 0, len(ids))
	for _, id := range ids {
		row, err := u.writeRow(ctx, snapshot, id)
		if err != nil {
			return nil, err
		}
		if row != nil && row.Type == domain.ChatTypeFolder {
			written = append(written, *row)
		}
	}
	return written, nil
}

// writeRow saves one row the plan touched, sending it down whichever command
// matches what the plan actually decided about it: a re-parented row has its
// placement written whole, a merely renumbered one has its index written and
// nothing else. A densify can therefore never restate a parent — and every
// parent in the snapshot came from the projection, one of them being stale being
// a routine consequence of the write before this one, not a rare interleaving.
func (u *chatFolderUsecase) writeRow(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (*domain.Chat, error) {
	row := snapshot.placedRow(id)
	if row == nil {
		return nil, nil
	}
	if !snapshot.plan.Reparented(id) {
		updated, err := u.chats.SetOrder(ctx, id, row.Order)
		if err != nil {
			return nil, fmt.Errorf("agent chat folder: order %s: %w", id, err)
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
) int {
	if requested != nil {
		return *requested
	}
	if origin == destination {
		return snapshot.plan.IndexOf(destination, id)
	}
	return snapshot.plan.NextSlot(destination)
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
