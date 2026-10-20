// Package turn (file conversation.go) answers one question the hook ingress
// asks of every event: is this the chat's conversation, or another one the
// provider pushed down the same wire?
package turn

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type hookSessionKey struct{}

// hookSession is the chat's bound session with one provider, as the hook being
// ingested read it, and the channel that hook arrived on.
type hookSession struct {
	chatID, provider, sessionID string
	channel                     engineagents.Channel
}

// withHookSession reads runner's chat's bound session once for the hook being
// ingested: the identity filter judges against it and everything the hook
// records names it.
func (t *Turns) withHookSession(
	ctx context.Context, runner engineagents.Runner, channel engineagents.Channel,
) (context.Context, string, error) {
	sessionID, err := t.readBoundSession(ctx, runner.CurrentChatID, runner.ProviderID)
	if err != nil {
		return ctx, "", fmt.Errorf("agent: ingest hook: %w", err)
	}
	h := hookSession{chatID: runner.CurrentChatID, provider: runner.ProviderID, sessionID: sessionID, channel: channel}
	return context.WithValue(ctx, hookSessionKey{}, h), sessionID, nil
}

// hookChannel is the channel the hook being ingested arrived on; "" outside one.
func hookChannel(ctx context.Context) engineagents.Channel {
	h, _ := ctx.Value(hookSessionKey{}).(hookSession)
	return h.channel
}

// namesAnotherConversation reports whether ev names a session other than the
// chat's bound one: a child thread or an internal session sharing the wire.
// session_start is the announcement, never filtered; an unbound chat filters nothing.
func namesAnotherConversation(bound string, ev engineagents.CanonicalEvent) bool {
	if ev.Kind == engineagents.HookSessionStart || ev.SessionID == "" {
		return false
	}
	return bound != "" && ev.SessionID != bound
}

// boundSession is chatID's bound session with provider: the one the hook being
// ingested read, else read now for a write outside a hook. A failed read names
// no session.
func (t *Turns) boundSession(ctx context.Context, chatID, provider string) string {
	if h, ok := ctx.Value(hookSessionKey{}).(hookSession); ok && h.chatID == chatID && h.provider == provider {
		return h.sessionID
	}
	sessionID, err := t.readBoundSession(ctx, chatID, provider)
	if err != nil {
		slog.WarnContext(ctx, "agent: read the chat's bound session", "chat_id", chatID, "err", err)
	}
	return sessionID
}

func (t *Turns) readBoundSession(ctx context.Context, chatID, provider string) (string, error) {
	if chatID == "" {
		return "", nil
	}
	st, err := t.ledger.State(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("read the chat's bound session: %w", err)
	}
	if c, ok := st.Conversation(provider); ok && c.State == ledger.ConversationBound {
		return c.SessionID, nil
	}
	return "", nil
}
