package chat

import "context"

// SetChatPermissionLevel overrides one chat's level for the rest of its
// lifetime, independent of the global default (see DefaultPermissionLevel).
// Refuses a level the chat's CURRENT provider does not declare — never
// clamps to a neighboring one, per the same rule the switcher's own options
// list is built from: if a provider cannot reach a level, it is not offered
// for that provider at all.
func (u *Usecase) SetChatPermissionLevel(
	ctx context.Context,
	chatID string,
	level string,
) error {
	return u.conversations.SetChatPermissionLevel(ctx, chatID, level)
}
