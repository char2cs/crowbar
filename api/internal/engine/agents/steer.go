package agents

import (
	"encoding/json"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func (a *agent) PromptSteer(text string) (string, bool) {
	ps := a.spec.Presentation.PromptSubmit
	if ps == nil || ps.Steer == nil || a.PromptIsCommand(text) {
		return "", false
	}
	framed, err := json.Marshal(strings.Replace(ps.Steer.Frame, "{message}", text, 1))
	if err != nil {
		return "", false
	}
	return strings.Replace(ps.Steer.Reply, "{message_json}", string(framed), 1), true
}

func (a *agent) PromptIsCommand(text string) bool {
	ps := a.spec.Presentation.PromptSubmit
	if ps == nil || ps.Steer == nil {
		return false
	}
	for _, prefix := range ps.Steer.SkipPrefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func (a *agent) ToolDenial(reason string) (string, bool) {
	ev, ok := a.spec.Events[spec.HookToolPre]
	if !ok || ev.Reply["deny"] == "" {
		return "", false
	}
	quoted, err := json.Marshal(reason)
	if err != nil {
		return "", false
	}
	return strings.Replace(ev.Reply["deny"], "{reason_json}", string(quoted), 1), true
}
