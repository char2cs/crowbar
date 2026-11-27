package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type OwningChats interface {
	MintOwningChat(
		ctx context.Context,
		chatID string,
		parentWorkspaceID string,
	) (string, error)
	AttachOwningWorkspace(
		ctx context.Context,
		chatID string,
		ws domain.Workspace,
	) error
	DiscardOwningChat(
		ctx context.Context,
		chatID string,
	) error
}

var ErrNoOwningChats = errors.New("project import: no owning-chat surface wired; refusing to create a chat-less workspace")

func (u *projectImport) createOwnedWorkspace(
	ctx context.Context,
	in workspace.CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	if u.owningChats == nil {
		return domain.Workspace{}, ErrNoOwningChats
	}
	chatID, err := u.owningChats.MintOwningChat(ctx, "", in.ParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("mint owning chat: %w", err)
	}
	ws, err := u.deps.Workspaces.Create(ctx, in, now)
	if err != nil {
		return domain.Workspace{}, u.discardOwningChat(ctx, chatID, err)
	}
	if nErr := u.mintWorkspaceNode(ctx, ws.ID); nErr != nil {
		return domain.Workspace{}, u.discardUnownedWorkspace(ctx, chatID, ws.ID, nErr)
	}
	if aErr := u.owningChats.AttachOwningWorkspace(ctx, chatID, ws); aErr != nil {
		return domain.Workspace{}, u.discardUnownedWorkspace(ctx, chatID, ws.ID, aErr)
	}
	return ws, nil
}

func (u *projectImport) mintWorkspaceNode(
	ctx context.Context,
	wsID string,
) error {
	if u.deps.Nodes == nil {
		return ErrNoNodesWired
	}
	if _, err := u.deps.Nodes.Create(ctx, wsID, tree.KindWorkspace, "", 0); err != nil {
		return fmt.Errorf("mint workspace node: %w", err)
	}
	return nil
}

func (u *projectImport) discardOwningChat(
	ctx context.Context,
	chatID string,
	cause error,
) error {
	if err := u.owningChats.DiscardOwningChat(ctx, chatID); err != nil {
		slog.WarnContext(ctx, "project import: discard the owning chat of a workspace that was never created",
			"chat_id", chatID, "err", err)
	}
	return cause
}

func (u *projectImport) discardUnownedWorkspace(
	ctx context.Context,
	chatID string,
	workspaceID string,
	cause error,
) error {
	if err := u.deps.Workspaces.Delete(ctx, workspaceID); err != nil {
		slog.WarnContext(ctx, "project import: discard the workspace no chat came to own",
			"workspace_id", workspaceID, "err", err)
	}
	if u.deps.Nodes != nil {
		if err := u.deps.Nodes.Forget(ctx, workspaceID); err != nil {
			slog.WarnContext(ctx, "project import: discard the node row of a workspace no chat came to own",
				"workspace_id", workspaceID, "err", err)
		}
	}
	return u.discardOwningChat(ctx, chatID, cause)
}
