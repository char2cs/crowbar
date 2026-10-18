// Package turn (file conversation.go) answers one question the hook ingress
// asks of every event: is this the chat's conversation, or another one the
// provider pushed down the same wire?
package turn

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// namesAnotherConversation reports whether ev names a session other than the
// chat's bound one with this provider: a child thread or an internal session
// sharing the connection. session_start is the announcement, never filtered.
func (t *Turns) namesAnotherConversation(
	ctx context.Context,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) bool {
	if ev.Kind == engineagents.HookSessionStart || ev.SessionID == "" {
		return false
	}
	mine := t.boundSession(ctx, runner.CurrentChatID, runner.ProviderID)
	return mine != "" && ev.SessionID != mine
}

// boundSession is chatID's bound session with provider; "" when there is none,
// which filters nothing. Nil-safe: guard tests build a bare &Turns{}.
func (t *Turns) boundSession(ctx context.Context, chatID, provider string) string {
	if t.ledger == nil || chatID == "" {
		return ""
	}
	st, err := t.ledger.State(ctx, chatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: read the chat's bound session", "chat_id", chatID, "err", err)
		return ""
	}
	if c, ok := st.Conversation(provider); ok && c.State == ledger.ConversationBound {
		return c.SessionID
	}
	return ""
}
