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
	// HookWarning is a warning the CLI prints in its own transcript.
	HookWarning     = "warning"
	HookPermission  = "permission"
	HookCompactPre  = "compact_pre"
	HookCompactPost = "compact_post"
	HookSessionEnd  = "session_end"
	HookTelemetry   = "telemetry"

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

	// The kinds a background command is: a shell, or a monitor that reports the
	// lines it prints.
	ShellKindShell   = "shell"
	ShellKindMonitor = "monitor"

	// HookTerminalInput is input written to the process of a running tool call: the
	// call outlived the moment it returned.
	HookTerminalInput = "terminal_input"

	// HookIdle is the provider reporting that it is doing nothing; it ends the
	// running turn, so a descriptor maps it only where no turn-close follows it.
	HookIdle = "idle"

	// HookPlanUpdate is the agent restating its own to-do list for this turn.
	HookPlanUpdate = "plan_update"

	// HookPlanStep is one step of that list created or changed alone.
	HookPlanStep = "plan_step"

	// HookSubagentResult is a subagent handing its result back to its parent,
	// which the parent then answers in a turn of its own.
	HookSubagentResult = "subagent_result"

	// HookDiffUpdate is the newest complete unified diff for the current turn.
	HookDiffUpdate = "diff_update"

	// HookMessageComplete is the whole text of one finished assistant message.
	HookMessageComplete = "message_complete"
)
