package hierarchy

import (
	"context"
	"errors"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ImportedBranch struct {
	ChatID            string
	RepoID            string
	ProjectID         string
	RepoPath          string
	RemoteURL         string
	Branch            string
	ParentWorkspaceID string
	ParentBranch      string
	ForceLocked       bool
}

type OwningChats interface {
	ImportBranchAsChat(
		ctx context.Context,
		in ImportedBranch,
	) (chatID, workspaceID string, err error)
	MintOwningChat(
		ctx context.Context,
		chatID string,
		parentWorkspaceID string,
	) (string, error)
	OwnerOf(
		ctx context.Context,
		workspaceID string,
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

var ErrNoOwningChats = errors.New("hierarchy: no owning-chat surface wired; refusing to create a chat-less workspace")

func (u *hierarchyUsecase) discardOwningChat(
	ctx context.Context,
	chatID string,
	cause error,
) error {
	if err := u.owningChats.DiscardOwningChat(ctx, chatID); err != nil {
		slog.WarnContext(ctx, "import: discard the owning chat of a workspace that was never created",
			"chat_id", chatID, "err", err)
	}
	return cause
}

func (u *hierarchyUsecase) discardUnownedWorkspace(
	ctx context.Context,
	chatID string,
	ws domain.Workspace,
	cause error,
) error {
	if err := u.removeOne(ctx, ws, u.repoRefFor(ctx, ws.RepoID), nil); err != nil {
		slog.WarnContext(ctx, "import: discard the workspace no chat came to own",
			"workspace_id", ws.ID, "err", err)
	}
	return u.discardOwningChat(ctx, chatID, cause)
}
