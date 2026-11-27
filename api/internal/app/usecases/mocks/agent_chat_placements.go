package mocks

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type AgentChatPlacements struct {
	Rows                     []domain.Chat
	Nodes                    *NodePlacements
	Purged                   []string
	Noted                    []LineageNote
	Minted                   []string
	MintedSurfaces           []string
	MintedProviders          []string
	DefaultProvider          string
	Started                  []StartCall
	SpawnedOwnWorktree       []StartCall
	OwnWorktreeBranches      []string
	SpawnedImportedWorktree  []StartCall
	ImportedBranches         []string
	AttachedWorkspaces       []PlacementWrite
	ListErr                  error
	GetErr                   error
	LoadErr                  error
	SetErr                   error
	OrderErr                 error
	PurgeErr                 error
	PurgeNotFoundID          string
	NoteErr                  error
	MintErr                  error
	StartErr                 error
	SpawnOwnWorktreeErr      error
	SpawnImportedWorktreeErr error
	AttachWorkspaceErr       error
	ImportedWorkspace        domain.Workspace
	SetCalls                 int
	MissingID                string
	Placed                   []PlacementWrite
	Ordered                  []OrderWrite
	Stale                    map[string]domain.Chat
	OrderNotFoundIDs         map[string]bool
	NextID                   string
	Moves                    MoveLedger
}

type PlacementWrite struct {
	ChatID   string
	ParentID string
	Order    int
}

type OrderWrite struct {
	ChatID string
	Order  int
}

func NewAgentChatPlacements() *AgentChatPlacements {
	return &AgentChatPlacements{}
}

func (s *AgentChatPlacements) ListByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	rows := make([]domain.Chat, 0, len(s.Rows))
	for _, c := range s.Rows {
		if c.WorkspaceID == workspaceID && c.ID != s.MissingID {
			rows = append(rows, s.projected(c))
		}
	}
	return rows, nil
}

func (s *AgentChatPlacements) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	rows := make([]domain.Chat, 0, len(s.Rows))
	for _, c := range s.Rows {
		if c.ID != s.MissingID {
			rows = append(rows, s.projected(c))
		}
	}
	return rows, nil
}

func (s *AgentChatPlacements) projected(
	row domain.Chat,
) domain.Chat {
	stale, ok := s.Stale[row.ID]
	if !ok {
		return row
	}
	return stale
}

func (s *AgentChatPlacements) Get(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return s.GetChat(ctx, id)
}

func (s *AgentChatPlacements) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	if s.GetErr != nil {
		return domain.Chat{}, s.GetErr
	}
	for _, c := range s.Rows {
		if c.ID == id {
			return s.projected(c), nil
		}
	}
	return domain.Chat{}, problem.ErrNotFound
}

func (s *AgentChatPlacements) LoadChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	if s.LoadErr != nil {
		return domain.Chat{}, s.LoadErr
	}
	for _, c := range s.Rows {
		if c.ID == id {
			return c, nil
		}
	}
	return domain.Chat{}, problem.ErrNotFound
}

func (s *AgentChatPlacements) SetPlacement(
	ctx context.Context,
	chatID string,
	parentID string,
	order int,
) (domain.Chat, error) {
	s.SetCalls++
	if s.SetErr != nil {
		return domain.Chat{}, s.SetErr
	}
	s.Placed = append(s.Placed, PlacementWrite{ChatID: chatID, ParentID: parentID, Order: order})
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].ParentID = parentID
			s.Rows[i].Order = order
			return s.Rows[i], nil
		}
	}
	return domain.Chat{}, nil
}

func (s *AgentChatPlacements) SetOrder(
	ctx context.Context,
	chatID string,
	order int,
) (domain.Chat, error) {
	if s.OrderErr != nil {
		return domain.Chat{}, s.OrderErr
	}
	if s.OrderNotFoundIDs[chatID] {
		return domain.Chat{}, fmt.Errorf("set order: %w", problem.ErrNotFound)
	}
	s.Ordered = append(s.Ordered, OrderWrite{ChatID: chatID, Order: order})
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].Order = order
			return s.Rows[i], nil
		}
	}
	return domain.Chat{}, nil
}

func (s *AgentChatPlacements) parentOf(
	chatID string,
) string {
	if s.Nodes != nil {
		for _, n := range s.Nodes.Rows {
			if n.ID == chatID {
				return n.ParentID
			}
		}
	}
	for _, c := range s.Rows {
		if c.ID == chatID {
			return c.ParentID
		}
	}
	return ""
}
