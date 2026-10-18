package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
)

// ResumeChat revives a dormant chat's last provider. Split out of lifecycle.go
// (responsibility, not size) once lastActiveProviderID's own fix landed there.
func (rs *Runners) ResumeChat(
	ctx context.Context,
	chatID string,
) (string, error) {
	park, release, err := rs.spawns.Acquire(ctx, chatID)
	if err != nil {
		return "", err
	}
	defer release()

	live, ok, err := rs.awaitDormant(park, chatID)
	if err != nil {
		return "", parkErr(park, fmt.Errorf("agent: resume chat: %w", err))
	}
	if ok {
		return live.ID, nil
	}
	providerID, err := rs.lastActiveProviderID(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: resume chat: %w", err)
	}
	// The gate is already held: call the inner body, never SwitchProvider itself.
	defer rs.enterPhase(ctx, chatID, snapshot.PhaseStarting)()
	return rs.switchProviderLocked(ctx, park, chatID, providerID)
}

// lastActiveProviderID answers "who comes back" for a dormant chat: the chat's
// own durable vendor (domain.Chat.ProviderID), restated on every placement and
// backfilled at boot for rows that predate it. A chat with none has genuinely
// never had a CLI placed on it, reported as ErrChatProviderUnknown — never as a
// missing RUNNER, which is both expected here and not what failed.
func (rs *Runners) lastActiveProviderID(ctx context.Context, chatID string) (string, error) {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}
	if chat.ProviderID == "" {
		return "", ErrChatProviderUnknown
	}
	return chat.ProviderID, nil
}
