package usecases

import (
	"context"
	"fmt"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type hierarchyOwningChats struct {
	tree  agentusecase.TreeUsecase
	chats chatsByWorkspace
}

type chatsByWorkspace interface {
	ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.Chat, error)
}

func (h hierarchyOwningChats) OwnerOf(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	rows, err := h.chats.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("owning chat of %s: %w", workspaceID, err)
	}
	owner, _ := domain.ResolveOwningChat(rows)
	return owner.ID, nil
}

func (h hierarchyOwningChats) ImportBranchAsChat(
	ctx context.Context,
	in workspace.ImportedBranch,
) (string, string, error) {
	return h.tree.ImportBranchAsChat(ctx, in.ChatID, agentusecase.ImportSpec{
		RepoID:            in.RepoID,
		ProjectID:         in.ProjectID,
		RepoPath:          in.RepoPath,
		RemoteURL:         in.RemoteURL,
		Branch:            in.Branch,
		ParentWorkspaceID: in.ParentWorkspaceID,
		ParentBranch:      in.ParentBranch,
		ForceLocked:       in.ForceLocked,
	})
}

func (h hierarchyOwningChats) MintOwningChat(
	ctx context.Context,
	chatID string,
	parentWorkspaceID string,
) (string, error) {
	return h.tree.MintOwningChat(ctx, chatID, parentWorkspaceID)
}

func (h hierarchyOwningChats) AttachOwningWorkspace(
	ctx context.Context,
	chatID string,
	ws domain.Workspace,
) error {
	return h.tree.AttachOwningWorkspace(ctx, chatID, ws)
}

func (h hierarchyOwningChats) DiscardOwningChat(
	ctx context.Context,
	chatID string,
) error {
	return h.tree.DiscardOwningChat(ctx, chatID)
}
