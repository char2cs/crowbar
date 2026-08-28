package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
)

// SetChatPermissionLevel overrides one chat's level for the rest of its
// lifetime, independent of the global default (see DefaultPermissionLevel).
// The override is in-memory only, exactly like the level itself.
func (u *Usecase) SetChatPermissionLevel(
	ctx context.Context,
	chatID string,
	level permission.Level,
) error {
	if !permission.Valid(level) {
		return fmt.Errorf("%w: unknown permission level %q", apperr.ErrInvalidArgument, level)
	}
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return fmt.Errorf("agent: set chat permission level: chat: %w", err)
	}
	u.permissionLevels.Set(chatID, level)
	return nil
}
