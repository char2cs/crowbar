package tree

import (
	"context"
	"errors"
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

// loadChat resolves a chat and refuses one anchored to another workspace. It is
// a NOT-FOUND rather than a cross-workspace refusal: the caller addressed a row
// that does not exist in the scope it asked in, and answering otherwise would
// tell it that a chat it may not touch exists.
//
// A folder id can no longer reach this door at all (2026-09-08
// sidebar-placement-unification Task 8): a folder is a domain.Folder row now,
// home-scoped or repo-scoped alike, never a Chats.LoadChat result — the
// chat/folder verb split that used to need a Type check here is now a
// resolution-door split instead (Rename/Move/Delete resolve through
// u.folders.FindByKey first, tree.go).
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
// Every FOLDER, home (RepoID=="") or repo-scoped (RepoID!="") alike since
// Task 8, is Node/Folder-backed now — rows beyond the raw ListChats() read
// are merged in from Node.ListByParent + Folder instead of ChatTypeFolder
// rows — see mergeForest. includeRepoPhantoms is passed true: a repo's own
// header row IS a legitimate sibling at project home, the one level folder
// CRUD's own global read has to plan against alongside home AND repo-scoped
// folders both.
//
// Folder CRUD (Create/Move/Delete) has no workspace, and therefore no project,
// to resolve a repo scope from — CreateInput/MoveInput carry no project id at
// all — so this passes mergeForest a nil repoMemberIDs (no filter). A
// bare-root folder operation can therefore still renumber another project's
// repo Node row sharing that container; a real, deliberately deferred gap
// (SDD review fix round 3), unlike PlaceChat's identical risk, which
// workspaceSnapshotAround below DOES close, because it has a real
// homeWorkspaceID in hand to resolve a project from.
//
// A workspaceAnchorType subject (PlaceWorkspace, 2026-09-09) is Node-backed
// for exactly the same reason a folder is — see subjectIsNodeBacked. So is an
// ordinary fork's subject: PlaceWorkspace resolves it to its OWNING CHAT
// (place_workspace.go's own nodeID doc), a real domain.Chat whose Type is
// ChatTypeBranch, not the workspaceAnchorType stand-in — omitting it here
// left writeRow send that chat's reorder through Chat.SetOrder (the legacy
// field nothing reads any more) while the Node this route actually placed —
// and every other reader reads back — never moved, caught live: dragging an
// ordinary fork past a sibling PATCHed 200 and visibly stayed put. Every
// other caller passes a folder subject or none at all, so this only changes
// behaviour for PlaceWorkspace's own call.
func (u *chatFolderUsecase) globalSnapshotAround(
	ctx context.Context,
	subject domain.Chat,
) (*treeSnapshot, error) {
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: snapshot: %w", err)
	}
	merged, homeIDs, err := u.mergeForest(ctx, rows, true, nil, nil)
	if err != nil {
		return nil, err
	}
	subjectIsNodeBacked := subject.ID != "" &&
		(subject.Type == domain.ChatTypeFolder || subject.Type == domain.ChatTypeBranch ||
			subject.Type == workspaceAnchorType)
	return buildHomeSnapshot(merged, subject, homeIDs, subjectIsNodeBacked), nil
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
// The empty workspace is a BUBBLE's own scope (model spec §3.1: a chat with
// no workspace at all) — the SUBJECT never routes through Node here (a
// bubble's placement is always Chat-based, ownWorktree's own mid-creation
// scope), but folder SIBLINGS it densifies against still might: a bubble can
// be dropped at the panel root alongside a real folder, and once folders
// left Chat entirely (2026-09-08 sidebar-placement-unification Task 5 for
// home-scoped, Task 8 for repo-scoped too) they no longer arrive through
// ListByWorkspace("") at all — mergeForest folds them in the same way it
// does for every other scope, just without ever marking the bubble ITSELF as
// Node-backed. includeRepoPhantoms is false: a bubble never densified
// against a Repository row before this migration either (Repository was
// never a Chat row), and nothing here changes that.
//
// For every OTHER workspace — home (Task 5) AND repo-scoped alike (Task 8)
// — the folder pass is replaced entirely by mergeForest: every folder is
// Node/Folder-backed now, not a Chat row, and a chat's own ParentID/Order
// need the SAME Node correction loadChat already applies (see
// correctHomePlacement) — a bare ListByWorkspace read alone would serve a
// chat's stale, write-once-at-creation Chat fields the moment its placement
// is Node-backed. isHomeWorkspace's answer decides ONLY whether repo
// phantoms participate (see mergeForest's own doc) — the merge itself runs
// either way.
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
		merged, nodeIDs, err := u.mergeForest(ctx, rows, false, nil, nil)
		if err != nil {
			return nil, err
		}
		snap := newTreeSnapshot(corrected(merged, subject))
		snap.homeIDs = nodeIDs
		return snap, nil
	}
	home, err := u.isHomeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return u.homeSnapshotAround(ctx, workspaceID, home, rows, subject)
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
			// A densify plans from one snapshot, but SetOrder lands on the live
			// aggregate: a concurrent delete can purge this very row between the
			// two. It needs no order because it no longer exists, so that race is
			// not this write's failure to report.
			if errors.Is(err, apperr.ErrNotFound) {
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
