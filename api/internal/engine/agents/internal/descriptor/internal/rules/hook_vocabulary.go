package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type hookVocabulary struct{}

func (hookVocabulary) Name() string { return "hook_vocabulary" }

// Check requires the two mappings without which a chat cannot exist at all: the
// conversation id a session announces, and the assistant text a turn ends with.
// Everything else in the hook vocabulary is optional and degrades to absent.
func (hookVocabulary) Check(d *spec.Descriptor) error {
	if d.Hooks.Format == "" {
		return invalid(d.ID, "missing hooks.format")
	}
	if d.Hooks.Events[spec.HookSessionStart]["session_id"] == "" {
		return invalid(d.ID, "hooks.events.session_start must map session_id")
	}
	if d.Hooks.Events[spec.HookTurnStop]["message"] == "" {
		return invalid(d.ID, "hooks.events.turn_stop must map message")
	}
	return nil
}
