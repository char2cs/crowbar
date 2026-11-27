package chat

import "context"

func (u *Usecase) SetChatPermissionLevel(
	ctx context.Context,
	chatID string,
	level string,
) error {
	return u.conversations.SetChatPermissionLevel(ctx, chatID, level)
}
