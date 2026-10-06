package agents

import (
	"encoding/json"
	"strings"
)

// PromptSteer renders text as the turn-end hook stdout that delivers it into a
// running turn. ok is false when the provider declares no steering, or text
// opens with a prefix the descriptor names as a command.
func (a *agent) PromptSteer(text string) (string, bool) {
	ps := a.spec.Presentation.PromptSubmit
	if ps == nil || ps.Steer == nil {
		return "", false
	}
	for _, prefix := range ps.Steer.SkipPrefixes {
		if strings.HasPrefix(text, prefix) {
			return "", false
		}
	}
	framed, err := json.Marshal(strings.Replace(ps.Steer.Frame, "{message}", text, 1))
	if err != nil {
		return "", false
	}
	return strings.Replace(ps.Steer.Reply, "{message_json}", string(framed), 1), true
}
