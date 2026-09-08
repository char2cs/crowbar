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

	// HookToolOutputDelta is a running tool's output as it is produced. Same
	// live channel, same never-recorded contract.
	HookToolOutputDelta = "tool_output_delta"

	HookTurnFailed = "turn_failed"

	// HookIdle is the provider reporting that it is doing nothing. It is NOT a
	// turn close — it routinely precedes one — and only ever arms a reconcile.
	HookIdle = "idle"
)
