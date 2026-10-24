package ingress

import (
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// boundSession is the chat's bound session with provider; "" when none is.
func boundSession(st ledger.State, provider string) string {
	if c, ok := st.Conversation(provider); ok && c.State == ledger.ConversationBound {
		return c.SessionID
	}
	return ""
}

// namesAnotherConversation reports whether ev names a session other than the
// chat's bound one: a nested conversation or an internal session sharing the
// wire. session_start is the announcement, never judged; an unbound chat
// judges nothing.
func namesAnotherConversation(bound string, ev engineagents.CanonicalEvent) bool {
	if ev.Kind == engineagents.HookSessionStart || ev.SessionID == "" {
		return false
	}
	return bound != "" && ev.SessionID != bound
}
