package evaluator

import "github.com/char2cs/crowbar/api/internal/engine/flow/def"

// Evaluate returns the target state name for the given event fired in currentState.
// Returns ("", false) if no matching transition exists for this event.
func Evaluate(d def.FlowDefinition, currentState string, event string) (string, bool) {
	for _, state := range d.States {
		if state.Name != currentState {
			continue
		}
		for _, t := range state.Transitions {
			if t.On == event {
				return t.To, true
			}
		}
		return "", false
	}
	return "", false
}
