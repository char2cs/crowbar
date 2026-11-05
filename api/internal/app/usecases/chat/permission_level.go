package chat

import "context"

// SetChatPermissionLevel overrides one chat's level for the rest of its lifetime,
// independent of the global default. Refuses a level the chat's CURRENT provider does
// not declare rather than clamping, matching the switcher's offered options.
func (u *Usecase) SetChatPermissionLevel(
	ctx context.Context,
	chatID string,
	level string,
) error {
	return u.conversations.SetChatPermissionLevel(ctx, chatID, level)
}
