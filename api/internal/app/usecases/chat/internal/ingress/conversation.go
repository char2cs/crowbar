package ingress

import (
	"slices"

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
// chat's bound one, sharing its wire; an unbound chat judges nothing. Over hooks
// a session_start is the CLI's announcement, never judged; an api connection's
// own sessions bind as it opens them, so one it reports starting is a child's.
func namesAnotherConversation(bound string, ev engineagents.CanonicalEvent, channel engineagents.Channel) bool {
	if ev.SessionID == "" || ev.Kind == engineagents.HookSessionStart && channel != engineagents.ChannelAPI {
		return false
	}
	return bound != "" && ev.SessionID != bound
}

// awaitsBind reports whether ev, of a conversation no subagent is bound to,
// may belong to one still starting. An ask is never held: its sender waits.
func awaitsBind(st ledger.State, ev engineagents.CanonicalEvent) bool {
	if ev.Kind == engineagents.HookPermission || ev.Kind == engineagents.HookElicitation {
		return false
	}
	return startingSubagent(st)
}

// startingSubagent reports whether a subagent of the chat is still starting:
// spawned, its conversation not yet bound.
func startingSubagent(st ledger.State) bool {
	return slices.ContainsFunc(st.Subagents(), func(a ledger.Subagent) bool { return a.Status == ledger.SubagentStarting })
}
