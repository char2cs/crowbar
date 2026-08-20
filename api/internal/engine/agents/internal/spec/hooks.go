package spec

// Canonical hook kinds. A descriptor maps each one it supports to the payload
// paths that carry Crowbar's vocabulary; a kind a descriptor omits simply never
// fires, which is how a provider that lacks a concept degrades to absent rather
// than to wrong.
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

	// HookToolFail is a tool finishing UNSUCCESSFULLY, and it is a kind of its own
	// rather than a status on tool_post because claude fires PostToolUseFailure
	// INSTEAD OF PostToolUse (measured against claude 2.1.234 on 2026-08-17). A
	// provider mapping only tool_post therefore never learns that a failed tool
	// stopped running, and the call reads as running until the turn sweeps it.
	HookToolFail = "tool_fail"

	// HookElicitation is an MCP server asking the human a question THROUGH the CLI.
	// Unlike AskUserQuestion — which is a tool, and so arrives as tool_pre plus a
	// permission — this is a hook event in its own right (measured against claude
	// 2.1.234 on 2026-08-17), so nothing but a canonical kind can observe it.
	HookElicitation = "elicitation"

	// HookMessageDelta is a chunk of what the agent is SAYING, delivered while it
	// says it.
	//
	// It exists because turn_stop.message is the LAST message of a turn and nothing
	// else. An agent that speaks, calls a tool, and speaks again produced two
	// messages and reported one, so everything before the final tool call was lost
	// — the defect this kind closes. Measured against claude 2.1.236: a turn with
	// ALPHA -> Bash -> OMEGA delivers both as separate messages here, while Stop
	// carried only OMEGA.
	//
	// Deltas are INCREMENTS, not cumulative snapshots, and Index is contiguous from
	// zero within each MessageID. Concatenating one message's deltas in Index order
	// reproduces the provider's own report of it byte for byte.
	//
	// A provider that declares nothing here keeps exactly its previous behaviour:
	// the last message of each turn, from turn_stop.
	HookMessageDelta = "message_delta"

	// HookTurnFailed is a turn that ENDED BADLY, with the provider's own reason.
	//
	// It is not turn_stop with a flag: the two are mutually exclusive on the wire.
	// A turn that fails fires this and NEVER fires turn_stop, so a chat mapping
	// only turn_stop is left with a turn nothing closes — indistinguishable from an
	// agent still working. Measured against claude 2.1.236, which reports
	// authentication, billing, rate_limit, overloaded, invalid_request,
	// model_not_found, server_error, max_output_tokens and prompt-too-long through
	// it. Those are precisely the endings that otherwise read as a hang.
	//
	// It does NOT cover a human INTERRUPT. That was measured too, and claude fires
	// no hook of any kind for one — see the interrupt detection in the usecase.
	HookTurnFailed = "turn_failed"
)

// HookSpec is the read side: how to parse this CLI's hook payloads (format) and
// where each Crowbar vocabulary field sits inside each canonical event's payload
// (events[canonical][vocab] = dotted path).
type HookSpec struct {
	Format string                       `yaml:"format"`
	Events map[string]map[string]string `yaml:"events"`

	// RequirePayloadFields names payload paths that EVERY hook of the CLI's own
	// user-facing conversation carries. A payload missing any of them is not a
	// conversation Crowbar hosts and is DROPPED before the reducer sees it.
	//
	// It exists because a CLI's hooks are not only its user's. Verified live
	// against codex 0.146.0 with `[features] memories = true`: its Memory Writing
	// Agent runs as an INTERNAL session firing SessionStart / UserPromptSubmit /
	// Stop / SessionEnd with a fresh session id and `source: startup` —
	// byte-identical to a real /new, so nothing in the move vocabulary can tell
	// them apart. What DOES tell them apart is that they are not conversations at
	// all: no rollout on disk (`transcript_path: null`). A session Crowbar cannot
	// resume and cannot hand off was never hostable.
	//
	// The guard is per-PAYLOAD, not per-event: dropping only the announcement
	// would leave the internal session's user_prompt and turn_stop to route by
	// the runner's placement into the USER'S chat.
	//
	// A provider that names nothing here keeps exactly its previous behaviour.
	RequirePayloadFields []string `yaml:"require_payload_fields"`
}

// Event returns the field map for a canonical kind, and whether it is declared.
func (h HookSpec) Event(canonical string) (map[string]string, bool) {
	fields, ok := h.Events[canonical]
	return fields, ok
}
