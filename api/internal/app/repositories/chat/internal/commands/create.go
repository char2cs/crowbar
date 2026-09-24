package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Create seeds a new AgentChat: an identity, a workspace and a clock, and
// nothing else. It carries no segment, no provider and no terminal session
// because a chat does not own the process talking to it — the runner does, and
// it is Started as its own aggregate (agentrunner). A chat can therefore be
// minted by the reducer (a /clear that lands on an unknown conversation) without
// any process fact being invented for it.
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
	// Still not a process fact: the runner is a separate aggregate and this
	// mint starts nothing. "" is what a chat minted before the field carries,
	// and what a reducer-minted chat (a /clear landing on an unknown
	// conversation) carries until its runner's own move restates it.
	ProviderID string
	Now        time.Time
}

func (c Create) AggregateID() string  { return c.ID }
func (c Create) EventName() string    { return "agentchat.created." + c.ID }
func (c Create) ShouldSnapshot() bool { return false }

func (c Create) Validate(current *domain.Chat) error {
	if current != nil {
		return fmt.Errorf("create agent chat: exists: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" {
		return fmt.Errorf("create agent chat: missing ids: %w", asynxModels.ErrValidation)
	}
	if !validChatType(c.Type) {
		return fmt.Errorf("create agent chat: invalid type: %w", asynxModels.ErrValidation)
	}
	return nil
}

// validChatType no longer accepts ChatTypeFolder (2026-09-08
// sidebar-placement-unification Task 8) or ChatTypeBranch (Task 9): a folder
// is a domain.Folder row now, home-scoped or repo-scoped alike, and a
// workspace's own position is its Node{Kind:workspace} row — neither is ever
// minted or retyped as a Chat aggregate any more.
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
		ID:             c.ID,
		WorkspaceID:    c.WorkspaceID,
		RepoID:         c.RepoID,
		Type:           c.Type,
		Surface:        c.Surface,
		ProviderID:     c.ProviderID,
		CreatedAt:      c.Now,
		LastActivityAt: c.Now,
	}
}
