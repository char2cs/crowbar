// Package runner (file capabilities.go) answers what a CHAT can actually be
// asked to do right now — as opposed to what its provider declares somewhere.
//
// A provider-scoped capability cannot tell two chats of the same provider
// apart, and two chats of the same provider are exactly what the surfaces
// model produces. The house rule these serve is absence, never a dead
// control: a gauge that can never move and a button that always errors are
// both worse than nothing on screen.
package runner

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TelemetryOnChatSurface reports whether chatID can receive its provider's
// usage reports on the surface it is on RIGHT NOW.
//
// It matters because the report store is DURABLE: a chat that earned one on a
// surface whose channel carries usage, and then moved to one that does not,
// went on serving that same number forever. Measured for codex, whose usage
// is an app-server notification and appears in no recorded hooks payload —
// see TestRegression_CodexReportsNoUsageOnItsHooksChannel.
//
// Any resolution failure answers TRUE. This gate exists to hide a number that
// cannot be true, never to hide one that can: a chat whose descriptor or
// worktree momentarily fails to resolve must not lose a gauge over it.
func (rs *Runners) TelemetryOnChatSurface(ctx context.Context, chatID string) bool {
	chat, agent, err := rs.chatCapabilityContext(ctx, chatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: resolve the chat's telemetry surface (keeping the gauge)",
			"chat_id", chatID, "err", err)
		return true
	}
	return agent.TelemetryOnSurface(chat.Surface)
}

// ErrCompactionOffSurface refuses compaction asked for from the provider's
// OWN terminal. It is Crowbar's native-chat affordance, not a provider
// capability the TUI is missing: on that surface the user types the
// provider's own gesture themselves, and Crowbar has no business drawing a
// second control for it. Provider-independent — claude included.
//
// No server-side READ counterpart, deliberately: the rule is the SURFACE and
// nothing else, the client already has it on every chat DTO, and a round trip
// to be told what it can see would be a second authority on one fact. The
// control is gated there (agent-chat-view.tsx's onTerminalSurface); this is
// what makes the gate enforced rather than merely honoured.
var ErrCompactionOffSurface = fmt.Errorf(
	"agent: compaction is offered on Crowbar's own chat, not the provider's terminal: %w",
	apperr.ErrUnprocessable,
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
