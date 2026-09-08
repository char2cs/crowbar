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
// A FOLDER row reached through this door is refused the same way, mirroring
// load() above: the chat verbs and the folder verbs apply opposite rules to the
// subtree they take (a chat delete cascades, a folder delete promotes), so an id
// arriving through the wrong verb must be told it names nothing rather than
// quietly getting the other rule. The workspace comparison alone does not
// separate them — a folder carries no workspace, so it matched the repo-scoped
// mount's empty :wsId exactly.
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
	if chat.Type == domain.ChatTypeFolder {
		return domain.Chat{}, fmt.Errorf("agent chat folder: %s is a folder: %w", chatID, apperr.ErrNotFound)
	}
	if chat.WorkspaceID != workspaceID {
		return domain.Chat{}, fmt.Errorf(
			"agent chat folder: chat %s is not in workspace %s: %w", chatID, workspaceID, apperr.ErrNotFound,
		)
	}
	return u.correctHomePlacement(ctx, chat)
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
//
// For the home scope (2026-09-08 sidebar-placement-unification Task 5), rows
// beyond the repo-scoped ListChats() read are merged in from Node.ListByParent
// + Folder instead of ListByWorkspace("") + ChatTypeFolder rows — see
// mergeHomeForest.
//
// Folder CRUD (Create/Move/Delete) has no workspace, and therefore no project,
// to resolve a repo scope from — CreateInput/MoveInput carry no project id at
// all — so this passes mergeHomeForest a nil repoMemberIDs (no filter). A
// bare-root folder operation can therefore still renumber another project's
// repo Node row sharing that container; a real, deliberately deferred gap
// (SDD review fix round 3), unlike PlaceChat's identical risk, which
// workspaceSnapshotAround below DOES close, because it has a real
// homeWorkspaceID in hand to resolve a project from.
func (u *chatFolderUsecase) globalSnapshotAround(
	ctx context.Context,
	subject domain.Chat,
) (*treeSnapshot, error) {
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: %w", err)
	}
	merged, homeIDs, err := u.mergeHomeForest(ctx, rows, nil)
	if err != nil {
		return nil, err
	}
	subjectIsHome := subject.ID != "" && subject.Type == domain.ChatTypeFolder && subject.RepoID == ""
	return buildHomeSnapshot(merged, subject, homeIDs, subjectIsHome), nil
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
//
// The folder pass is SKIPPED for the empty workspace, which is a BUBBLE's own
// scope (model spec §3.1: a chat with no workspace at all). A folder carries no
// workspace either, so ListByWorkspace("") already returned every one of them —
// appending them again put each folder in the plan twice, and a level counted
// twice hands the next row a slot past the end of it (see NextSlot).
//
// For the home workspace (2026-09-08 sidebar-placement-unification Task 5),
// the folder pass is replaced entirely by mergeHomeForest: home folders are
// Node/Folder-backed now, not Chat rows, and a home chat's own ParentID/Order
// need the SAME Node correction loadChat already applies (see
// correctHomePlacement) — a bare ListByWorkspace read alone would serve a
// home chat's stale, write-once-at-creation Chat fields.
func (u *chatFolderUsecase) workspaceSnapshotAround(
	ctx context.Context,
	workspaceID string,
	subject domain.Chat,
) (*treeSnapshot, error) {
	rows, err := u.chats.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: chats: %w", err)
	}
	if workspaceID == "" {
		return newTreeSnapshot(corrected(rows, subject)), nil
	}
	home, err := u.isHomeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if home {
		return u.homeSnapshotAround(ctx, workspaceID, rows, subject)
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
//
// A row in snapshot.freshIDs is force-included even when the generic
// tree.Tree plan reports it as NOT dirty: a home-scoped chat's very first
// placement (right after MintChat) is already sitting at the exact
// ParentID/Order its OWN row carried into the snapshot (a zero-value bubble,
// "" / 0) whenever it happens to land back at the front of an empty or
// tied level, so SetParent/Reorder record no CHANGE for it — invisible to
// the plan's own numeric diff, the identical coincidence project.go's
// forceReparentWrite/finalIndexOf exists to catch for a reparenting repo.
// Fresh means "no Node row exists yet at all," which owes a Nodes.Create
// regardless of whether anything about its ParentID/Order actually moved.
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
		if row != nil && row.Type == domain.ChatTypeFolder {
			written = append(written, *row)
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

// writeRow saves one row the plan touched, sending it down whichever command
// matches what the plan actually decided about it: a re-parented row has its
// placement written whole, a merely renumbered one has its index written and
// nothing else. A densify can therefore never restate a parent — and every
// parent in the snapshot came from the projection, one of them being stale being
// a routine consequence of the write before this one, not a rare interleaving.
//
// A row snapshot.homeIDs marks (2026-09-08 sidebar-placement-unification
// Task 5 — a home-scoped chat, a home folder, or a repo phantom, see
// mergeHomeForest) is dispatched to writeHomeNode instead: its POSITION lives
// on Node now, not Chat.ParentID/.Order.
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
