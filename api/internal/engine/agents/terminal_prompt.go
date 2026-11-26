package agents

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"

func (a *agent) MatchTerminalPrompt(screen string) (TerminalPrompt, bool) {
	return protocol.MatchTerminalPrompt(a.spec, screen)
}
