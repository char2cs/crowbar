package mocks

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// AgentChatPlacements is a fake chat-tree Chats AND Agent over the sidebar forest's rows.
// Purged and Noted keep call order: a cascade erases children first, and a note's lineage
// is permanent in the chat's conversation, so the recorded ancestors are asserted.
type AgentChatPlacements struct {
	Rows []domain.Chat
	// Nodes optionally cross-references the NodePlacements fake wired over the same
	// tree.Usecase; nil means Chat rows only.
	Nodes  *NodePlacements
	Purged []string
	Noted  []LineageNote
	Minted []string
	// MintedSurfaces is the landing surface each MintChat call asked for, in
	// the same order as Minted — "" for the provider's own default.
	MintedSurfaces []string
	// MintedProviders is the provider each MintChat recorded, in Minted order ("" for a
	// placeholder): a chat minted without one cannot name its vendor when dormant.
	MintedProviders []string
	Started         []StartCall
	// SpawnedOwnWorktree records each SpawnChatWithOwnWorktree call as a StartCall, proving
	// the placement landed before the spawn.
	SpawnedOwnWorktree []StartCall
	// OwnWorktreeBranches is the branch name each SpawnedOwnWorktree call asked
	// for, in the same order — "" for the server-generated-name case, mirroring
	// ImportedBranches below.
	OwnWorktreeBranches []string
	// SpawnedImportedWorktree records each SpawnChatWithImportedWorktree call in
	// the same shape, and ImportedSpecs the branch each one asked for — the
	// import counterpart of SpawnedOwnWorktree above.
	SpawnedImportedWorktree []StartCall
	ImportedBranches        []string
	// AttachedWorkspaces records each AttachWorkspace call as chatID->workspaceID,
	// in order, so a test can prove the chat existed before the workspace it owns.
	AttachedWorkspaces []PlacementWrite
	ListErr            error
	GetErr             error
	LoadErr            error
	SetErr             error
	OrderErr           error
	PurgeErr           error
	// PurgeNotFoundID makes PurgeChat answer problem.ErrNotFound for this id (still recorded),
	// as for a tree chat with no conversation aggregate; PurgeErr fails every call instead.
	PurgeNotFoundID string
	NoteErr         error
	MintErr         error
	StartErr        error
	// SpawnOwnWorktreeErr fails SpawnChatWithOwnWorktree, the same way StartErr
	// fails StartRunner.
	SpawnOwnWorktreeErr error
	// SpawnImportedWorktreeErr fails SpawnChatWithImportedWorktree, and
	// AttachWorkspaceErr fails AttachWorkspace, the same way StartErr fails
	// StartRunner.
	SpawnImportedWorktreeErr error
	AttachWorkspaceErr       error
	// ImportedWorkspace is the workspace SpawnChatWithImportedWorktree hands
	// back. Its zero value is an ordinary unlocked worktree; a test that wants
	// the row typed as a BRANCH sets Status to locked here.
	ImportedWorkspace domain.Workspace
	SetCalls          int
	MissingID         string
	// Placed and Ordered record the two placement writes SEPARATELY, because the
	// difference between them is the contract: a renumber may write an index and
	// must be unable to write a parent.
	Placed  []PlacementWrite
	Ordered []OrderWrite
	// Stale is the PROJECTED placement answered while the read model lags the log: the
	// ordinary state right after every placement write.
	Stale map[string]domain.Chat
	// OrderNotFoundIDs makes SetOrder answer problem.ErrNotFound, as when a concurrent
	// request purged the chat between the densify read and the write.
	OrderNotFoundIDs map[string]bool
	// NextID is the id the next MintChat hands back, so a test can name the chat a
	// create is about to make instead of discovering it from the return value.
	NextID string
}

// PlacementWrite is one recorded call to SetPlacement: a row the caller MOVED,
// and where to.
type PlacementWrite struct {
	ChatID   string
	ParentID string
	Order    int
}

// OrderWrite is one recorded call to SetOrder: a row a densify renumbered, and
// the index it was given. It carries no parent, which is the whole point.
type OrderWrite struct {
	ChatID string
	Order  int
}

// NewAgentChatPlacements returns an empty AgentChatPlacements.
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

// ListChats answers every row the daemon knows — folders included, since they
// are the same aggregate now — the read folder CRUD plans against.
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

// projected answers with the row as the READ MODEL has it, which is the row
// itself unless a test is holding the projection behind the log.
func (s *AgentChatPlacements) projected(
	row domain.Chat,
) domain.Chat {
	stale, ok := s.Stale[row.ID]
	if !ok {
		return row
	}
	return stale
}

// Get is GetChat under the tree usecase's shorter name — see EventStore.Get.
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

// LoadChat is the log fold: it answers with the row as it actually stands,
// whatever the projection is still serving.
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

// SetOrder writes the index and leaves the parent exactly as it stands, like the
// command it stands in for: the aggregate applies it to the row folded from the
// log, so there is no parent in the call to be stale.
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

// parentOf answers chatID's CURRENT parent for StartCall.ParentAtStart: Nodes
// first, as production dispatches, since a Node-backed placement never
// touches Chat.ParentID; the Chat row for a bubble.
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
