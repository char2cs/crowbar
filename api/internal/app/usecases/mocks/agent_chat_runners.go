package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/branchimport"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// StartCall is one recorded OpenChat call. ParentAtStart, read at call time, is what
// tells place-then-start from start-then-place: the spawn reads it to decide inheritance.
type StartCall struct {
	ChatID        string
	ProviderID    string
	ParentAtStart string
}

// LineageNote is one recorded call to NoteThreadLineage: which chat was told,
// and what it was told it now reads.
type LineageNote struct {
	ChatID    string
	Ancestors []string
}

func (s *AgentChatPlacements) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	if s.PurgeErr != nil {
		return s.PurgeErr
	}
	s.Purged = append(s.Purged, chatID)
	kept := s.Rows[:0]
	for _, c := range s.Rows {
		if c.ID != chatID {
			kept = append(kept, c)
		}
	}
	s.Rows = kept
	if chatID == s.PurgeNotFoundID {
		return problem.ErrNotFound
	}
	return nil
}

func (s *AgentChatPlacements) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	if s.NoteErr != nil {
		return s.NoteErr
	}
	s.Noted = append(s.Noted, LineageNote{ChatID: chatID, Ancestors: ancestors})
	return nil
}

func (s *AgentChatPlacements) MintChat(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	surface string,
) (string, error) {
	if s.MintErr != nil {
		return "", s.MintErr
	}
	if chatID == "" {
		chatID = s.NextID
	}
	s.Minted = append(s.Minted, workspaceID)
	s.MintedSurfaces = append(s.MintedSurfaces, surface)
	s.MintedProviders = append(s.MintedProviders, providerID)
	s.Rows = append(s.Rows, domain.Chat{
		ID:          chatID,
		Type:        domain.ChatTypeChat,
		WorkspaceID: workspaceID,
		ProviderID:  providerID,
		Surface:     surface,
	})
	return chatID, nil
}

func (s *AgentChatPlacements) OpenChat(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	if s.StartErr != nil {
		return "", s.StartErr
	}
	s.Started = append(s.Started, StartCall{
		ChatID:        chatID,
		ProviderID:    providerID,
		ParentAtStart: s.parentOf(chatID),
	})
	return "runner-" + chatID, nil
}

// SpawnChatWithOwnWorktree records the call and, on success, fills the row's WorkspaceID
// as the real one does, so a test can assert the row owns a workspace.
func (s *AgentChatPlacements) SpawnChatWithOwnWorktree(
	ctx context.Context,
	chatID string,
	providerID string,
	branch string,
) (string, string, error) {
	if s.SpawnOwnWorktreeErr != nil {
		return "", "", s.SpawnOwnWorktreeErr
	}
	s.SpawnedOwnWorktree = append(s.SpawnedOwnWorktree, StartCall{
		ChatID:        chatID,
		ProviderID:    providerID,
		ParentAtStart: s.parentOf(chatID),
	})
	s.OwnWorktreeBranches = append(s.OwnWorktreeBranches, branch)
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].WorkspaceID = "ws-child-" + chatID
		}
	}
	return "ws-child-" + chatID, "runner-" + chatID, nil
}

// SpawnChatWithImportedWorktree mirrors SpawnChatWithOwnWorktree for an existing branch;
// an empty providerID starts no runner, as in production.
func (s *AgentChatPlacements) SpawnChatWithImportedWorktree(
	_ context.Context,
	chatID string,
	providerID string,
	spec branchimport.Spec,
) (domain.Workspace, string, error) {
	if s.SpawnImportedWorktreeErr != nil {
		return domain.Workspace{}, "", s.SpawnImportedWorktreeErr
	}
	s.SpawnedImportedWorktree = append(s.SpawnedImportedWorktree, StartCall{
		ChatID:        chatID,
		ProviderID:    providerID,
		ParentAtStart: s.parentOf(chatID),
	})
	s.ImportedBranches = append(s.ImportedBranches, spec.Branch)
	ws := s.ImportedWorkspace
	if ws.ID == "" {
		ws.ID = "ws-import-" + chatID
	}
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].WorkspaceID = ws.ID
		}
	}
	if providerID == "" {
		return ws, "", nil
	}
	return ws, "runner-" + chatID, nil
}

// AttachWorkspace records the slot write and applies it to the held row.
func (s *AgentChatPlacements) AttachWorkspace(
	_ context.Context,
	chatID string,
	workspaceID string,
) error {
	if s.AttachWorkspaceErr != nil {
		return s.AttachWorkspaceErr
	}
	s.AttachedWorkspaces = append(s.AttachedWorkspaces, PlacementWrite{
		ChatID:   chatID,
		ParentID: workspaceID,
	})
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].WorkspaceID = workspaceID
			s.Rows[i].OwnsWorkspace = workspaceID != ""
		}
	}
	return nil
}
