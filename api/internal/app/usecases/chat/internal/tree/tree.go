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
// BackfillOwningChats'. Nothing else here reads a workspace at all.
func New(
	chats Chats,
	agent Agent,
	work *inflight.Work,
	workspaces WorkspaceGitStatus,
	roster WorkspaceRoster,
) Usecase {
	return &chatFolderUsecase{
		chats:      chats,
		agent:      agent,
		work:       work,
		workspaces: workspaces,
		roster:     roster,
	}
}

func (u *chatFolderUsecase) ListInRepo(
	ctx context.Context,
	repoID string,
) ([]domain.Chat, error) {
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: list in repo: %w", err)
	}
	out := make([]domain.Chat, 0, len(rows))
	for _, row := range rows {
		if row.Type == domain.ChatTypeFolder {
			out = append(out, row)
		}
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
	snapshot, err := u.globalSnapshot(ctx)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	if cErr := u.checkFolderContainer(ctx, snapshot, in.ParentID); cErr != nil {
		return domain.Chat{}, nil, cErr
	}
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}
	minted, err := u.chats.Create(ctx, agentchat.CreateInput{
		ID:   id,
		Type: domain.ChatTypeFolder,
		Now:  time.Now(),
	})
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: create %s: %w", id, err)
	}
	created, written, err := u.placeNewFolder(ctx, snapshot, id, name, in.ParentID, minted.CreatedAt)
	if err != nil {
		return domain.Chat{}, nil, u.discardFolder(ctx, id, err)
	}
	return created, written, nil
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

func (u *chatFolderUsecase) Rename(
	ctx context.Context,
	id string,
	name string,
) (domain.Chat, error) {
	clean, err := cleanName(name)
	if err != nil {
		return domain.Chat{}, err
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

func (u *chatFolderUsecase) Move(
	ctx context.Context,
	id string,
	in MoveInput,
) (domain.Chat, []domain.Chat, error) {
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
	if mErr := u.checkFolderMove(ctx, snapshot, id, destination); mErr != nil {
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

func (u *chatFolderUsecase) Delete(
	ctx context.Context,
	id string,
) ([]domain.Chat, error) {
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
