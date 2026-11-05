package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// Create seeds a new AgentChat: an identity, a workspace and a clock. It carries
// no terminal session because a chat does not own the process talking to it:
// its ledger records each runner_started instead.
type Create struct {
	ID          string
	WorkspaceID string
	// RepoID is meaningful only when Type is ChatTypeFolder — see
	// domain.Chat.RepoID's own doc. Left "" for every other type.
	RepoID string
	Type   domain.ChatType
	// Surface is the chat's landing VIEW — see domain.Chat.Surface. "" is
	// the provider's own default and is what every chat minted before this
	// field existed carries.
	Surface string
	// ProviderID is the vendor the chat is BORN on — see domain.Chat.ProviderID.
	// This mint starts nothing; "" is what a chat minted before the field carries.
	ProviderID string
	Now        time.Time
}

func (c Create) AggregateID() string  { return c.ID }
func (c Create) EventName() string    { return "agentchat.created." + c.ID }
func (c Create) ShouldSnapshot() bool { return false }

func (c Create) Validate(current *domain.Chat) error {
	if current != nil {
		return &problem.Error{Code: problem.IDConflict, Message: fmt.Sprintf("chat %s already exists", c.ID)}
	}
	if c.ID == "" {
		return fmt.Errorf("create agent chat: missing ids: %w", asynxModels.ErrValidation)
	}
	if !validChatType(c.Type) {
		return fmt.Errorf("create agent chat: invalid type: %w", asynxModels.ErrValidation)
	}
	return nil
}

// validChatType refuses ChatTypeFolder and ChatTypeBranch: a folder is a
// tree.Folder row and a workspace's position is its Node{Kind:workspace} row,
// so neither is minted or retyped as a Chat aggregate.
func validChatType(t domain.ChatType) bool {
	switch t {
	case domain.ChatTypeChat, domain.ChatTypeWorkflow:
		return true
	case domain.ChatTypeFolder, domain.ChatTypeBranch:
		return false
	default:
		return false
	}
}

func (c Create) EmitEvent(_ *domain.Chat) domain.Chat {
	return domain.Chat{
		ID:          c.ID,
		WorkspaceID: c.WorkspaceID,
		RepoID:      c.RepoID,
		Type:        c.Type,
		Surface:     c.Surface,
		ProviderID:  c.ProviderID,
		CreatedAt:   c.Now,
	}
}
