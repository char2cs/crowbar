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
	HookWarning      = "warning"
	HookPermission   = "permission"
	HookCompactPre   = "compact_pre"
	HookCompactPost  = "compact_post"
	HookSessionEnd   = "session_end"
	HookTelemetry    = "telemetry"

	HookToolFail = "tool_fail"

	HookElicitation = "elicitation"

	HookMessageDelta = "message_delta"

	HookReasoningDelta = "reasoning_delta"

	HookToolOutputDelta = "tool_output_delta"

	HookTurnFailed = "turn_failed"

	FailureAuth       = "auth"
	FailureLimit      = "limit"
	FailureOverloaded = "overloaded"

	ShellKindShell   = "shell"
	ShellKindMonitor = "monitor"

	HookTerminalInput = "terminal_input"

	HookIdle = "idle"

	HookPlanUpdate = "plan_update"

	HookPlanStep = "plan_step"

	HookSubagentResult = "subagent_result"

	HookDiffUpdate = "diff_update"

	HookMessageComplete = "message_complete"
)
