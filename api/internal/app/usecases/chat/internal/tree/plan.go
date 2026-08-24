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

// load resolves a folder and refuses one belonging to another workspace, which
// is a NOT-FOUND rather than a cross-workspace refusal: the caller addressed a
// row that does not exist in the scope it asked in, and any other answer would
// confirm the existence of a row it may not touch. The check lives here rather
// than in the handler so every caller shares one rule.
func (u *chatFolderUsecase) load(
	ctx context.Context,
	workspaceID string,
	id string,
) (domain.ChatFolder, error) {
	row, err := u.folders.FindByKey(ctx, id)
	if err != nil {
		return domain.ChatFolder{}, fmt.Errorf("agent chat folder: get %s: %w", id, err)
	}
	if row == nil || row.WorkspaceID != workspaceID {
		return domain.ChatFolder{}, fmt.Errorf("agent chat folder: %s: %w", id, apperr.ErrNotFound)
	}
	return *row, nil
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

// snapshot reads one workspace's folders and chats as of a single moment. Both
// reads are workspace-scoped: the folder query is pushed down to SQL, and the
// chat read model serves the workspace slice natively.
func (u *chatFolderUsecase) snapshot(
	ctx context.Context,
	workspaceID string,
) (*treeSnapshot, error) {
	return u.snapshotAround(ctx, workspaceID, domain.Chat{})
}

// snapshotAround is the same read with the SUBJECT's row corrected from the log
// fold the caller already holds.
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
func (u *chatFolderUsecase) snapshotAround(
	ctx context.Context,
	workspaceID string,
	subject domain.Chat,
) (*treeSnapshot, error) {
	folders, err := u.folders.FindWhere(ctx, domain.ChatFolder{WorkspaceID: workspaceID})
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: folders: %w", err)
	}
	chats, err := u.chats.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: chats: %w", err)
	}
	return newTreeSnapshot(folders, corrected(chats, subject)), nil
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
// among them, which the caller has to broadcast itself. The chat rows need no
// such handling: their write is an aggregate command, so the hub projection
// broadcasts each one on the way through.
func (u *chatFolderUsecase) persist(
	ctx context.Context,
	snapshot *treeSnapshot,
) ([]domain.ChatFolder, error) {
	ids := snapshot.plan.Dirty()
	written := make([]domain.ChatFolder, 0, len(ids))
	for _, id := range ids {
		row, err := u.writeRow(ctx, snapshot, id)
		if err != nil {
			return nil, err
		}
		if row != nil {
			written = append(written, *row)
		}
	}
	return written, nil
}

// writeRow saves one row the plan touched, sending a CHAT down whichever command
// matches what the plan actually decided about it: a re-parented chat has its
// placement written whole, a merely renumbered one has its index written and
// nothing else. A densify can therefore never restate a parent — and every
// parent in the snapshot came from the projection, one of them being stale being
// a routine consequence of the write before this one, not a rare interleaving.
//
// Folders need neither: they are read and written through the same synchronous
// table, so a folder read back after a folder write is already current.
func (u *chatFolderUsecase) writeRow(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (*domain.ChatFolder, error) {
	if row := snapshot.placedFolder(id); row != nil {
		if err := u.folders.Save(ctx, *row); err != nil {
			return nil, fmt.Errorf("agent chat folder: save %s: %w", id, err)
		}
		return row, nil
	}
	row := snapshot.placedChat(id)
	if row == nil {
		return nil, nil
	}
	if !snapshot.plan.Reparented(id) {
		if _, err := u.chats.SetOrder(ctx, id, row.Order); err != nil {
			return nil, fmt.Errorf("agent chat folder: order chat %s: %w", id, err)
		}
		return nil, nil
	}
	if _, err := u.chats.SetPlacement(ctx, id, row.ParentID, row.Order); err != nil {
		return nil, fmt.Errorf("agent chat folder: place chat %s: %w", id, err)
	}
	return nil, nil
}

// without drops the subject row from a written set, leaving the collateral the
// caller broadcasts alongside it.
func without(
	rows []domain.ChatFolder,
	id string,
) []domain.ChatFolder {
	out := make([]domain.ChatFolder, 0, len(rows))
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
