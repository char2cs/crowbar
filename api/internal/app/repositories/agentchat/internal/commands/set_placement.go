package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetPlacement writes where a chat sits in the Chats tree: the row it hangs off
// and its dense index within that sibling space.
//
// Unlike the workspace command of the same name, this DOES move lineage, and
// deliberately. A workspace's ParentID is a git fact — what a branch was forked
// from — so the sidebar's drag is barred from rewriting it. A chat's parent is
// not a fact about anything on disk: it IS the relationship, and the panel draws
// no other mark for it. Dragging a chat under another chat therefore makes it a
// thread of that chat, and dragging it back out makes it standalone again.
//
// ParentID names a chat or a folder, or "" for the panel root. Which of the two
// it names decides whether this row inherits context: a thread reads every
// ANCESTOR CHAT's turns, and folders are transparent to that walk.
//
// The command mutates fields and nothing else. Cycle refusal, the shared
// chat/folder sibling space and the densify all live in the usecase, which can
// see the whole tree; a command sees one aggregate and could not check them.
type SetPlacement struct {
	ID       string
	ParentID string
	Order    int
}

func (c SetPlacement) AggregateID() string {
	return c.ID
}

func (c SetPlacement) EventName() string {
	return "agentchat.placement_set." + c.ID
}

func (c SetPlacement) ShouldSnapshot() bool {
	return false
}

func (c SetPlacement) Validate(
	current *domain.AgentChat,
) error {
	if current == nil {
		return fmt.Errorf("set placement: no chat: %w", asynxModels.ErrValidation)
	}
	if c.ParentID == c.ID {
		return fmt.Errorf("set placement: chat under itself: %w", asynxModels.ErrValidation)
	}
	if c.Order < 0 {
		return fmt.Errorf("set placement: negative order: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetPlacement) EmitEvent(
	current *domain.AgentChat,
) domain.AgentChat {
	chat := *current
	chat.ParentID = c.ParentID
	chat.Order = c.Order
	return chat
}
