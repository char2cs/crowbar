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

// namesAnotherConversation reports whether ev names a session other than the chat's
// bound one. A hook session_start is the CLI's own announcement, never judged; over
// api, sessions bind as opened, so a reported start is a child's.
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

// handsBackUnbound reports whether ev hands back the result of a scope no
// subagent is bound to while one is starting: it may be that one's result.
func handsBackUnbound(st ledger.State, ev engineagents.CanonicalEvent) bool {
	if ev.Kind != engineagents.HookSubagentResult || ev.Subagent == nil || ev.Subagent.ID == "" {
		return false
	}
	_, bound := st.SubagentOf(ev.Subagent.ID)
	return !bound && startingSubagent(st)
}

// startingSubagent reports whether a subagent of the chat is still starting:
// spawned, its conversation not yet bound.
func startingSubagent(st ledger.State) bool {
	return slices.ContainsFunc(st.Subagents(), func(a ledger.Subagent) bool { return a.Status == ledger.SubagentStarting })
}
