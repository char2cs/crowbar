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
	// message_delta is optional, but a HALF-mapped one is worse than none. Its
	// three identity fields are what make a stream of increments reassemblable:
	// without message_id every delta of a turn merges into one message, and
	// without index a dropped chunk is a silent hole instead of a detectable gap.
	// A descriptor that maps the text and forgets those would look like it works
	// and would quietly corrupt what the agent is recorded as having said.
	if delta, ok := d.Hooks.Events[spec.HookMessageDelta]; ok {
		for _, field := range []string{"message_id", "index", "text"} {
			if delta[field] == "" {
				return invalid(d.ID, "hooks.events.message_delta must map %s", field)
			}
		}
	}
	// Same rule, same reason: turn_failed exists to say WHY a turn ended, and one
	// that cannot say why is turn_stop with extra steps.
	if failed, ok := d.Hooks.Events[spec.HookTurnFailed]; ok && failed["reason"] == "" {
		return invalid(d.ID, "hooks.events.turn_failed must map reason")
	}
	return nil
}
