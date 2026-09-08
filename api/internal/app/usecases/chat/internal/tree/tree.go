// Package tree owns the sidebar forest's organisation layer: the FOLDER rows a
// user files chats into, where each row hangs, and the dense sibling order
// every row kind SHARES — they interleave at every level and sort on one Order
// field within one ParentID.
//
// A folder is no longer its own table. `domain.Folder` (the old sidebar tree)
// and `domain.ChatFolder` (the old Chats-panel tree) have folded into one row —
// a `domain.Chat` whose Type is ChatTypeFolder — so every operation here reads
// and writes through the same chat repository a conversation row does.
//
// Its two deletes are deliberately opposite, and the asymmetry is the whole
// domain rule, unchanged by the retype. Deleting a FOLDER promotes what it held
// to the folder's own parent: a folder holds no conversation, so the chats
// outlive it. Deleting a CHAT-typed row takes its entire subtree: a thread
// exists to CONTINUE its parent — it reads that parent's turns — so leaving it
// behind would strand it reading a context that no longer exists.
//
// Nothing here reasons about processes. A chat's runner, its PTY and its ledger
// belong to the agent usecase; this one moves rows and, for the cascade, asks
// that usecase to erase each CHAT-typed row it has decided must go. A deleted
// FOLDER is erased directly through the chat repository instead (Chats.Forget)
// — it never had a runner or a ledger to tear down, so routing it through the
// agent usecase would be pure cost.
//
// Every operation reads its rows ONCE, plans the whole change in memory, and
// then writes only the rows that actually moved. That is not merely an
// optimisation: the chat read model is an asynchronous projection, so a
// re-read taken between a write and the renumber that follows it can still be
// serving the pre-write list. Planning from a single snapshot removes the race
// rather than papering over it with a barrier.
package tree

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type chatFolderUsecase struct {
	chats      Chats
	agent      Agent
	work       *inflight.Work
	workspaces WorkspaceGitStatus
	roster     WorkspaceRoster
	reaper     WorkspaceReaper
	holders    WorkspaceHolders
	// folders and nodes are the home-scoped (RepoID == "") folder/chat
	// placement surface (2026-09-08 sidebar-placement-unification Task 5) —
	// see Folders/Nodes' own docs (types.go). Repo-scoped folders/chats never
	// touch either; they keep going through chats above, unchanged.
	folders Folders
	nodes   Nodes
}

// New builds the tree usecase over the chat row repository and the agent
// usecase behind everything a chat is besides a row.
//
// The agent handle is required in both directions — this usecase decides which
// chats a delete takes and which chat a create is born under, and the agent
// usecase is the only thing that knows how to erase one, mint one, or start a
// CLI on one.
//
// work is the SAME in-flight tracker the agent usecase's own turn and runner
// components observe, not a second one: a move or delete refuses over the
// subtree it takes by asking it directly, so the answer can never lag behind
// what a hook just announced.
//
// workspaces is DeletePreview's seam onto the workspace layer; roster is
// BackfillOwningChats'; reaper is DeleteChat's, and it is REQUIRED rather than
// optional for the reason ChatTreeUsecase itself is: a delete wired without it
// would erase a chat and silently strand the worktree it owned, which is the
// bug this port exists to close. Making it a parameter puts that mis-wire in
// front of the compiler instead of in front of a user.
//
// holders is required for the same reason and guards the same verb from the
// opposite failure: the reaper alone will tear down whatever it is handed, so
// without the holder census DeleteChat cascades a worktree that surviving
// sibling chats are still working in. One port says how a workspace is torn
// down; the other says whether it may be.
// folders and nodes are the home-scoped folder/chat placement surface (see
// Folders/Nodes' own docs, types.go) — Task 5 of the 2026-09-08
// sidebar-placement-unification plan.
func New(
	chats Chats,
	agent Agent,
	work *inflight.Work,
	workspaces WorkspaceGitStatus,
	roster WorkspaceRoster,
	reaper WorkspaceReaper,
	holders WorkspaceHolders,
	folders Folders,
	nodes Nodes,
) Usecase {
	return &chatFolderUsecase{
		chats:      chats,
		agent:      agent,
		work:       work,
		workspaces: workspaces,
		roster:     roster,
		reaper:     reaper,
		holders:    holders,
		folders:    folders,
		nodes:      nodes,
	}
}

func (u *chatFolderUsecase) ListInRepo(
	ctx context.Context,
	repoID string,
) ([]domain.Chat, error) {
	if repoID == "" {
		return u.listHomeFolders(ctx)
	}
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: list in repo: %w", err)
	}
	out := make([]domain.Chat, 0, len(rows))
	for _, row := range rows {
		// repoID is "" for the project-home caller and a real repo id
		// otherwise — the SAME convention RepoID is stored under (see
		// domain.Chat.RepoID), so a plain equality is the whole boundary:
		// no other repo's (or home's) folders bleed in any more.
		if row.Type == domain.ChatTypeFolder && row.RepoID == repoID {
			out = append(out, row)
		}
	}
	return out, nil
}

// listHomeFolders is ListInRepo("")'s home-scoped body (2026-09-08
// sidebar-placement-unification Task 5): every domain.Folder with RepoID ==
// "", rendered as the same Chat-shaped view every other home read returns
// (see plan.go's homeFolderView) so the wire contract does not change.
func (u *chatFolderUsecase) listHomeFolders(
	ctx context.Context,
) ([]domain.Chat, error) {
	all, err := u.folders.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: list in repo: %w", err)
	}
	out := make([]domain.Chat, 0, len(all))
	for _, f := range all {
		if f.RepoID != "" {
			continue
		}
		row := homeFolderView(f, domain.Node{})
		if n, nerr := u.nodes.GetNode(ctx, f.ID); nerr == nil {
			row = homeFolderView(f, n)
		}
		out = append(out, row)
	}
	return out, nil
}

func (u *chatFolderUsecase) Create(
	ctx context.Context,
	in CreateInput,
) (domain.Chat, []domain.Chat, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	if in.RepoID == "" {
		return u.createHomeFolder(ctx, in, name)
	}
	snapshot, err := u.globalSnapshot(ctx)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	if cErr := u.checkFolderContainer(ctx, snapshot, in.RepoID, in.ParentID); cErr != nil {
		return domain.Chat{}, nil, cErr
	}
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}
	minted, err := u.chats.Create(ctx, agentchat.CreateInput{
		ID:     id,
		Type:   domain.ChatTypeFolder,
		RepoID: in.RepoID,
		Now:    time.Now(),
	})
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: create %s: %w", id, err)
	}
	created, written, err := u.placeNewFolder(ctx, snapshot, id, in.RepoID, name, in.ParentID, minted.CreatedAt)
	if err != nil {
		return domain.Chat{}, nil, u.discardFolder(ctx, id, err)
	}
	return created, written, nil
}

// createHomeFolder is Create's home-scoped (RepoID == "") body (2026-09-08
// sidebar-placement-unification Task 5): identity is a plain domain.Folder
// row, position is a domain.Node row, minted through the SAME
// snapshot/plan/persist densify every other Create call already runs —
// checkFolderContainer, guard walks and the write dispatch are entirely
// unchanged, only WHERE this one row's identity/position come from differs
// (see writeRow's home dispatch, plan.go).
func (u *chatFolderUsecase) createHomeFolder(
	ctx context.Context,
	in CreateInput,
	name string,
) (domain.Chat, []domain.Chat, error) {
	snapshot, err := u.globalSnapshot(ctx)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	if cErr := u.checkFolderContainer(ctx, snapshot, "", in.ParentID); cErr != nil {
		return domain.Chat{}, nil, cErr
	}
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}
	if err := u.folders.Save(ctx, domain.Folder{ID: id, Name: name, RepoID: ""}); err != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: create %s: %w", id, err)
	}
	target := snapshot.plan.NextSlot(in.ParentID)
	snapshot.add(domain.Chat{
		ID:       id,
		Type:     domain.ChatTypeFolder,
		Title:    name,
		RepoID:   "",
		ParentID: in.ParentID,
		Order:    target,
	})
	snapshot.homeIDs[id] = true
	snapshot.freshIDs[id] = true
	snapshot.plan.Reorder(in.ParentID, id, target)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, u.discardHomeFolder(ctx, id, err)
	}
	return *snapshot.placedRow(id), without(written, id), nil
}

// placeNewFolder names and places a just-minted folder: everything Create does
// AFTER the mint, kept in one function so a single discardFolder at its call
// site covers every way it can fail — naming or the densify that follows it —
// mirroring chats.go's CreateChat, whose discard likewise wraps its entire
// post-mint sequence rather than only the first step of it.
func (u *chatFolderUsecase) placeNewFolder(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
	repoID string,
	name string,
	parentID string,
	createdAt time.Time,
) (domain.Chat, []domain.Chat, error) {
	titled, err := u.chats.SetTitle(ctx, id, name, "user")
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: create %s: name: %w", id, err)
	}
	target := snapshot.plan.NextSlot(parentID)
	snapshot.add(domain.Chat{
		ID:        id,
		Type:      domain.ChatTypeFolder,
		Title:     titled.Title,
		RepoID:    repoID,
		ParentID:  parentID,
		Order:     target,
		CreatedAt: createdAt,
	})
	snapshot.plan.Reorder(parentID, id, target)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	return *snapshot.placedRow(id), without(written, id), nil
}

// discardFolder takes a just-minted folder back out when the create failed
// after minting it, and hands back the failure that caused it. The purge is
// best-effort and NEVER replaces the cause, mirroring chats.go's discard for
// the same reason: the user is told what actually failed.
func (u *chatFolderUsecase) discardFolder(
	ctx context.Context,
	id string,
	cause error,
) error {
	if err := u.chats.Forget(ctx, id); err != nil {
		return fmt.Errorf("%w (and cleanup failed: %v)", cause, err)
	}
	return cause
}

// discardHomeFolder is discardFolder's home-scoped counterpart: the Folder row
// is always taken back out (Save already ran), and the Node row is a
// best-effort Forget — persist may have failed before this row's own Create
// ever ran, and Forget on an unknown id is a tolerated no-op (mirrors
// project.go's importOneRepo rollback).
func (u *chatFolderUsecase) discardHomeFolder(
	ctx context.Context,
	id string,
	cause error,
) error {
	if err := u.folders.Delete(ctx, id); err != nil {
		return fmt.Errorf("%w (and cleanup failed: %v)", cause, err)
	}
	if err := u.nodes.Forget(ctx, id); err != nil {
		return fmt.Errorf("%w (and node cleanup failed: %v)", cause, err)
	}
	return cause
}

func (u *chatFolderUsecase) Rename(
	ctx context.Context,
	id string,
	name string,
) (domain.Chat, error) {
	clean, err := cleanName(name)
	if err != nil {
		return domain.Chat{}, err
	}
	if f, ferr := u.folders.FindByKey(ctx, id); ferr != nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: rename %s: %w", id, ferr)
	} else if f != nil && f.RepoID == "" {
		return u.renameHomeFolder(ctx, *f, clean)
	}
	if _, err := u.load(ctx, id); err != nil {
		return domain.Chat{}, err
	}
	renamed, err := u.chats.SetTitle(ctx, id, clean, "user")
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: rename %s: save: %w", id, err)
	}
	return renamed, nil
}

// renameHomeFolder is Rename's home-scoped body: the folder's NAME lives on
// domain.Folder, not a chat aggregate, so a rename is a plain field save —
// its placement is untouched, exactly like the repo-scoped path above.
func (u *chatFolderUsecase) renameHomeFolder(
	ctx context.Context,
	f domain.Folder,
	name string,
) (domain.Chat, error) {
	f.Name = name
	if err := u.folders.Save(ctx, f); err != nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: rename %s: save: %w", f.ID, err)
	}
	n, err := u.nodes.GetNode(ctx, f.ID)
	if err != nil {
		return homeFolderView(f, domain.Node{}), nil
	}
	return homeFolderView(f, n), nil
}

func (u *chatFolderUsecase) Move(
	ctx context.Context,
	id string,
	in MoveInput,
) (domain.Chat, []domain.Chat, error) {
	if f, ferr := u.folders.FindByKey(ctx, id); ferr != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: move %s: %w", id, ferr)
	} else if f != nil && f.RepoID == "" {
		return u.moveHomeFolder(ctx, *f, in)
	}
	current, err := u.load(ctx, id)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if mErr := u.checkFolderMove(ctx, snapshot, current.RepoID, id, destination); mErr != nil {
		return domain.Chat{}, nil, mErr
	}
	if wErr := guardNotWorking(subtreeIDsOf(id, snapshot.rows), u.work); wErr != nil {
		return domain.Chat{}, nil, wErr
	}
	u.replace(snapshot, id, current.ParentID, destination, in.Order)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	return *snapshot.placedRow(id), without(written, id), nil
}

// moveHomeFolder is Move's home-scoped body. The identity half (f) is already
// resolved by the caller; position comes off the live Node row rather than a
// Chat's frozen ParentID/Order, then runs through the SAME
// globalSnapshotAround/checkFolderMove/guardNotWorking/replace/persist chain
// the repo-scoped path uses — see mergeHomeForest (plan.go) for how that
// snapshot ends up seeing this row (and its repo/chat siblings) correctly.
func (u *chatFolderUsecase) moveHomeFolder(
	ctx context.Context,
	f domain.Folder,
	in MoveInput,
) (domain.Chat, []domain.Chat, error) {
	n, err := u.nodes.GetNode(ctx, f.ID)
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: move %s: node: %w", f.ID, err)
	}
	current := homeFolderView(f, n)
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if mErr := u.checkFolderMove(ctx, snapshot, f.RepoID, f.ID, destination); mErr != nil {
		return domain.Chat{}, nil, mErr
	}
	if wErr := guardNotWorking(subtreeIDsOf(f.ID, snapshot.rows), u.work); wErr != nil {
		return domain.Chat{}, nil, wErr
	}
	u.replace(snapshot, f.ID, current.ParentID, destination, in.Order)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	return *snapshot.placedRow(f.ID), without(written, f.ID), nil
}

func (u *chatFolderUsecase) Delete(
	ctx context.Context,
	id string,
) ([]domain.Chat, error) {
	if f, ferr := u.folders.FindByKey(ctx, id); ferr != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: %w", id, ferr)
	} else if f != nil && f.RepoID == "" {
		return u.deleteHomeFolder(ctx, *f)
	}
	current, err := u.load(ctx, id)
	if err != nil {
		return nil, err
	}
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return nil, err
	}
	if wErr := guardNotWorking(subtreeIDsOf(id, snapshot.rows), u.work); wErr != nil {
		return nil, wErr
	}
	if err := u.chats.Forget(ctx, id); err != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: %w", id, err)
	}
	snapshot.plan.Reparent(id, current.ParentID)
	snapshot.drop(id)
	snapshot.plan.Reorder(current.ParentID, "", -1)
	return u.persist(ctx, snapshot)
}

// deleteHomeFolder is Delete's home-scoped body: what f held is PROMOTED to
// f's own parent (never cascaded — folders hold no conversation, see the
// package doc), f's Folder row and Node row are both erased, and the level it
// sat in is closed up — the same shape Delete's repo-scoped body runs, over a
// Folder+Node-sourced row instead of a Chat one.
func (u *chatFolderUsecase) deleteHomeFolder(
	ctx context.Context,
	f domain.Folder,
) ([]domain.Chat, error) {
	n, err := u.nodes.GetNode(ctx, f.ID)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: node: %w", f.ID, err)
	}
	current := homeFolderView(f, n)
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return nil, err
	}
	if wErr := guardNotWorking(subtreeIDsOf(f.ID, snapshot.rows), u.work); wErr != nil {
		return nil, wErr
	}
	if err := u.folders.Delete(ctx, f.ID); err != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: %w", f.ID, err)
	}
	if err := u.nodes.Forget(ctx, f.ID); err != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: node: %w", f.ID, err)
	}
	snapshot.plan.Reparent(f.ID, current.ParentID)
	snapshot.drop(f.ID)
	snapshot.plan.Reorder(current.ParentID, "", -1)
	return u.persist(ctx, snapshot)
}
