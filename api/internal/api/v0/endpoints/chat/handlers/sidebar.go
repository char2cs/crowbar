package handlers

import (
	"context"
	"fmt"
	"slices"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Rows is one repo's or one project home's sidebar rows, as the list routes
// serve them; the projects stream scope's snapshot reads them through here.
type Rows struct {
	Workspaces []dto.WorkspaceDTO
	Chats      []dto.AgentChatDTO
	Folders    []dto.AgentChatDTO
}

// RepoRows is repoID's workspaces, chats and folders.
func (h *Handlers) RepoRows(ctx context.Context, projectID, repoID string) (Rows, error) {
	workspaces, err := h.workspacesIn(ctx, projectID, repoID)
	if err != nil {
		return Rows{}, err
	}
	rows, err := h.chatAndFolderRows(ctx, "", repoID)
	rows.Workspaces = workspaces
	return rows, err
}

// HomeRows is the chats and folders of the project home homeID.
func (h *Handlers) HomeRows(ctx context.Context, homeID string) (Rows, error) {
	return h.chatAndFolderRows(ctx, homeID, "")
}

func (h *Handlers) chatAndFolderRows(ctx context.Context, homeID, repoID string) (Rows, error) {
	chats, runtimes, err := h.chatsWithRuntimes(ctx, homeID, repoID)
	if err != nil {
		return Rows{}, fmt.Errorf("chat: sidebar chats: %w", err)
	}
	folders, err := h.listFolders(ctx, repoID, homeID)
	if err != nil {
		return Rows{}, fmt.Errorf("chat: sidebar folders: %w", err)
	}
	out := dto.AgentChatDTOList(folders, nil, nil)
	slices.SortFunc(out, compareFolderDTOs)
	return Rows{Chats: dto.AgentChatDTOList(chats, runtimes, nil), Folders: out}, nil
}
