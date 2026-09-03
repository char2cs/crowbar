// Package tree owns the Chats panel's tree: the AgentChatFolder rows
// a user files chats into, where each chat hangs, and the dense sibling order
// the two kinds SHARE — they interleave at every level and sort on one Order
// field within one ParentID.
//
// Its two deletes are deliberately opposite, and the asymmetry is the whole
// domain rule. Deleting a FOLDER promotes what it held to the folder's own
// parent: a folder holds no conversation, so the chats outlive it. Deleting a
// CHAT takes its entire subtree: a thread exists to CONTINUE its parent — it
// reads that parent's turns — so leaving it behind would strand it reading a
// context that no longer exists.
//
// Nothing here reasons about processes. A chat's runner, its PTY and its ledger
// belong to the agent usecase; this one moves rows and, for the cascade, asks
// that usecase to erase each chat it has decided must go.
//
// Every operation reads the workspace's rows ONCE, plans the whole change in
// memory, and then writes only the rows that actually moved. That is not merely
// an optimisation: the chat read model is an asynchronous projection, so a
// re-read taken between a write and the renumber that follows it can still be
// serving the pre-write list. Planning from a single snapshot removes the race
// rather than papering over it with a barrier.
package tree

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type chatFolderUsecase struct {
	folders Store
	chats   Chats
	agent   Agent
}

// New builds the chat-folder usecase over the folders table, the chat row
// repository, and the agent usecase behind everything a chat is besides a row.
//
// The chat handle is required because the two row kinds share one sibling space:
// renumbering a level without it would leave folders and chats holding
// independent, colliding indices. The agent handle is required in both
// directions — this usecase decides which chats a delete takes and which chat a
// create is born under, and the agent usecase is the only thing that knows how to
// erase one, mint one, or start a CLI on one.
func New(
	folders Store,
	chats Chats,
	agent Agent,
) Usecase {
	return &chatFolderUsecase{folders: folders, chats: chats, agent: agent}
}

func (u *chatFolderUsecase) ListInWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.ChatFolder, error) {
	rows, err := u.folders.FindWhere(ctx, domain.ChatFolder{WorkspaceID: workspaceID})
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: list in workspace: %w", err)
	}
	return rows, nil
}

func (u *chatFolderUsecase) Create(
	ctx context.Context,
	in CreateInput,
) (domain.ChatFolder, []domain.ChatFolder, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return domain.ChatFolder{}, nil, err
	}
	snapshot, err := u.snapshot(ctx, in.WorkspaceID)
	if err != nil {
		return domain.ChatFolder{}, nil, err
	}
	if cErr := u.checkContainer(ctx, snapshot, in.WorkspaceID, in.ParentID); cErr != nil {
		return domain.ChatFolder{}, nil, cErr
	}
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}
	target := snapshot.plan.NextSlot(in.ParentID)
	snapshot.add(domain.ChatFolder{
		ID:          id,
		WorkspaceID: in.WorkspaceID,
		ParentID:    in.ParentID,
		Name:        name,
		Order:       target,
	})
	snapshot.plan.Reorder(in.ParentID, id, target)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.ChatFolder{}, nil, err
	}
	return *snapshot.placedFolder(id), without(written, id), nil
}

func (u *chatFolderUsecase) Rename(
	ctx context.Context,
	workspaceID string,
	id string,
	name string,
) (domain.ChatFolder, error) {
	clean, err := cleanName(name)
	if err != nil {
		return domain.ChatFolder{}, err
	}
	current, err := u.load(ctx, workspaceID, id)
	if err != nil {
		return domain.ChatFolder{}, err
	}
	current.Name = clean
	if err := u.folders.Save(ctx, current); err != nil {
		return domain.ChatFolder{}, fmt.Errorf("agent chat folder: rename %s: save: %w", id, err)
	}
	return current, nil
}

func (u *chatFolderUsecase) Move(
	ctx context.Context,
	workspaceID string,
	id string,
	in MoveInput,
) (domain.ChatFolder, []domain.ChatFolder, error) {
	current, err := u.load(ctx, workspaceID, id)
	if err != nil {
		return domain.ChatFolder{}, nil, err
	}
	snapshot, err := u.snapshot(ctx, current.WorkspaceID)
	if err != nil {
		return domain.ChatFolder{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if mErr := u.checkMove(ctx, snapshot, current.WorkspaceID, id, destination); mErr != nil {
		return domain.ChatFolder{}, nil, mErr
	}
	u.replace(snapshot, id, current.ParentID, destination, in.Order)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.ChatFolder{}, nil, err
	}
	return *snapshot.placedFolder(id), without(written, id), nil
}

func (u *chatFolderUsecase) Delete(
	ctx context.Context,
	workspaceID string,
	id string,
) ([]domain.ChatFolder, error) {
	current, err := u.load(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	snapshot, err := u.snapshot(ctx, current.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if err := u.folders.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: %w", id, err)
	}
	snapshot.plan.Reparent(id, current.ParentID)
	snapshot.drop(id)
	snapshot.plan.Reorder(current.ParentID, "", -1)
	return u.persist(ctx, snapshot)
}
