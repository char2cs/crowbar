package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetType rewrites which KIND of row a chat is, leaving its id, placement,
// workspace and conversation exactly as they stand.
//
// It exists for one fact the taxonomy has to survive: what a row IS can change
// after the row is made. A workspace that was an ordinary worktree when its
// owning row was minted becomes a locked branch the moment the user locks it or
// the provider reports its branch protected, and the row that owns it is then a
// BRANCH row — the same row, playing a different part. Retyping it is what keeps
// "one workspace, one owning row" true across that change; minting a second row
// of the right type would leave two rows both claiming the workspace.
//
// A FOLDER is out of bounds in both directions. Folder rows and chat rows share
// one table but not one set of verbs — a folder delete PROMOTES what it held
// where a chat delete CASCADES into it — so a retype across that line would hand
// existing rows the opposite rule from the one they were filed under.
type SetType struct {
	ID   string
	Type domain.ChatType
}

func (c SetType) AggregateID() string {
	return c.ID
}

func (c SetType) EventName() string {
	return "agentchat.type_set." + c.ID
}

func (c SetType) ShouldSnapshot() bool {
	return false
}

func (c SetType) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("set type: no chat: %w", asynxModels.ErrValidation)
	}
	if !validChatType(c.Type) {
		return fmt.Errorf("set type: invalid type: %w", asynxModels.ErrValidation)
	}
	if c.Type == domain.ChatTypeFolder || current.Type == domain.ChatTypeFolder {
		return fmt.Errorf("set type: across the folder boundary: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetType) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	chat.Type = c.Type
	return chat
}
