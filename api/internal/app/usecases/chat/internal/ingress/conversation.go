package ingress

import (
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func boundSession(st ledger.State, provider string) string {
	if c, ok := st.Conversation(provider); ok && c.State == ledger.ConversationBound {
		return c.SessionID
	}
	return ""
}

func namesAnotherConversation(bound string, ev engineagents.CanonicalEvent, channel engineagents.Channel) bool {
	if ev.SessionID == "" || ev.Kind == engineagents.HookSessionStart && channel != engineagents.ChannelAPI {
		return false
	}
	return bound != "" && ev.SessionID != bound
}

func awaitsBind(st ledger.State, ev engineagents.CanonicalEvent) bool {
	if ev.Kind == engineagents.HookPermission || ev.Kind == engineagents.HookElicitation {
		return false
	}
	return startingSubagent(st)
}

func handsBackUnbound(st ledger.State, ev engineagents.CanonicalEvent) bool {
	if ev.Kind != engineagents.HookSubagentResult || ev.Subagent == nil || ev.Subagent.ID == "" {
		return false
	}
	_, bound := st.SubagentOf(ev.Subagent.ID)
	return !bound && startingSubagent(st)
}

func startingSubagent(st ledger.State) bool {
	return slices.ContainsFunc(st.Subagents(), func(a ledger.Subagent) bool { return a.Status == ledger.SubagentStarting })
}
