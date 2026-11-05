// Package runner (file capabilities.go) answers what a CHAT can do right now, which a
// provider-scoped capability cannot: two chats of one provider may be on different
// surfaces. The rule is absence, never a dead control.
package runner

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TelemetryOnChatSurface reports whether chatID can receive usage reports on its
// CURRENT surface; the store is durable, so a moved chat would serve a stale number.
// Any resolution failure answers TRUE: it hides only a number that cannot be true.
func (rs *Runners) TelemetryOnChatSurface(ctx context.Context, chatID string) bool {
	chat, agent, err := rs.chatCapabilityContext(ctx, chatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: resolve the chat's telemetry surface (keeping the gauge)",
			"chat_id", chatID, "err", err)
		return true
	}
	return agent.TelemetryOnSurface(chat.Surface)
}

// ErrCompactionOffSurface refuses compaction asked from the provider's OWN terminal,
// where the user types the provider's gesture themselves. The client gates the control
// off the surface it already has; this enforces that gate.
var ErrCompactionOffSurface = fmt.Errorf(
	"agent: compaction is offered on Crowbar's own chat, not the provider's terminal: %w",
	problem.ErrUnprocessable,
)

// chatCapabilityContext resolves the pair every chat-scoped capability needs:
// the chat itself (for its current surface) and the descriptor of whichever
// provider is on it.
func (rs *Runners) chatCapabilityContext(
	ctx context.Context, chatID string,
) (domain.Chat, engineagents.Agent, error) {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("chat: %w", err)
	}
	providerID, err := rs.conversations.ChatProviderID(ctx, chatID)
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("provider: %w", err)
	}
	cwdWorkspaceID, err := rs.cwdWorkspaceID(ctx, chat.ID, chat.WorkspaceID)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	crowbarHome, _, _, _, err := rs.ws.WorktreeDir(ctx, cwdWorkspaceID)
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("worktree dir: %w", err)
	}
	agent, err := rs.agents.Get(ctx, crowbarHome, providerID)
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("resolve descriptor: %w", err)
	}
	return chat, agent, nil
}
