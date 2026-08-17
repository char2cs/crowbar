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
