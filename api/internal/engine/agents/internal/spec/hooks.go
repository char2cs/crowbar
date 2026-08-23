package spec

const (
	HookSessionStart = "session_start"
	HookUserPrompt   = "user_prompt"
	HookTurnStop     = "turn_stop"
	HookToolPre      = "tool_pre"
	HookToolPost     = "tool_post"
	HookSubagentPre  = "subagent_pre"
	HookSubagentPost = "subagent_post"
	HookNotification = "notification"
	HookPermission   = "permission"
	HookCompactPre   = "compact_pre"
	HookCompactPost  = "compact_post"
	HookSessionEnd   = "session_end"
	HookTelemetry    = "telemetry"

	HookToolFail = "tool_fail"

	HookElicitation = "elicitation"

	HookMessageDelta = "message_delta"

	HookTurnFailed = "turn_failed"
)

type HookSpec struct {
	Format string                       `yaml:"format"`
	Events map[string]map[string]string `yaml:"events"`

	RequirePayloadFields []string `yaml:"require_payload_fields"`
}

func (h HookSpec) Event(canonical string) (map[string]string, bool) {
	fields, ok := h.Events[canonical]
	return fields, ok
}
