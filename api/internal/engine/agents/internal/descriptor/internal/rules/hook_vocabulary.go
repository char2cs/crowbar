package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type hookVocabulary struct{}

func (hookVocabulary) Name() string { return "hook_vocabulary" }

func (hookVocabulary) Check(d *spec.Descriptor) error {
	// A v3 descriptor's event table is validated against vocabulary.yaml by
	// schema.Validate, which is stricter than this and driven by data. This rule
	// covers only the v2 files, and dies with them.
	if d.IsV3() {
		return nil
	}
	if d.Hooks.Format == "" {
		return invalid(d.ID, "missing hooks.format")
	}
	if d.Hooks.Events[spec.HookSessionStart]["session_id"] == "" {
		return invalid(d.ID, "hooks.events.session_start must map session_id")
	}
	if d.Hooks.Events[spec.HookTurnStop]["message"] == "" {
		return invalid(d.ID, "hooks.events.turn_stop must map message")
	}

	if delta, ok := d.Hooks.Events[spec.HookMessageDelta]; ok {
		for _, field := range []string{"message_id", "index", "text"} {
			if delta[field] == "" {
				return invalid(d.ID, "hooks.events.message_delta must map %s", field)
			}
		}
	}

	if failed, ok := d.Hooks.Events[spec.HookTurnFailed]; ok && failed["reason"] == "" {
		return invalid(d.ID, "hooks.events.turn_failed must map reason")
	}
	return nil
}
