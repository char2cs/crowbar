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

	// HookReasoningDelta is the model thinking out loud. It rides the same live
	// channel as HookMessageDelta, tagged with a kind, and is never recorded as
	// the assistant's answer.
	HookReasoningDelta = "reasoning_delta"

	HookTurnFailed = "turn_failed"
)
