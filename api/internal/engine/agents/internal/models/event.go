package models

// CanonicalEvent is one provider hook, mapped into Crowbar's vocabulary. Every
// field is optional: a descriptor maps what its provider reports, and what it
// does not map stays zero. That is the whole degradation story — an older CLI, or
// a provider without the concept, lands on the same zero value as a descriptor
// that never declared the field.
//
// It deliberately carries NO provider `source` label. Claude reports
// source=clear where Codex reports source=startup for the very same event, so
// any branch on that vocabulary is provider-specific and will break on the next
// CLI. Carrying it would invite exactly that branch.
type CanonicalEvent struct {
	Kind      string
	SessionID string
	Message   string

	// AsyncWork is how many units of asynchronous work the CLI reports STILL
	// OUTSTANDING as of this event — a LEVEL it re-states every time, not a delta.
	//
	// Provider-agnostic by construction: the descriptor names the array whose
	// LENGTH is that level, and a provider that names nothing reports 0 forever.
	// The engine counts; it never learns what the entries mean.
	//
	// A level, because the edges are a lie. Measured against claude 2.1.212, the
	// SubagentStart/SubagentStop pair does NOT balance — the two hooks observe
	// different populations — so one run gave 4 starts against 9 stops and another
	// 3 starts against 0 stops. Counting edges drifts in BOTH directions: too many
	// stops clear the spinner early, too few strand it ON forever. A re-stated
	// level cannot drift, because every report overwrites the last wholesale.
	AsyncWork int

	// Model is the model the CLI actually resolved. It is the only reliable way to
	// learn that an org allowlist SUBSTITUTED the requested model: the substitute
	// is reported here, before any tokens are spent.
	Model string
	// Effort is the reasoning effort the CLI actually used, when it reports one.
	Effort string
	// Reason accompanies a session ending.
	Reason string

	Tool      *ToolEvent
	Subagent  *SubagentEvent
	Interrupt *InterruptEvent

	Raw map[string]any
}

// ToolEvent is one tool invocation or completion. Input and Result are the raw
// payload subtrees the descriptor pointed at, carried as bytes because they go to
// the content store and must never be inlined into an event log.
type ToolEvent struct {
	ID   string
	Name string
	// Target is the one field worth indexing across tools — the file path, the
	// command, the URL. A descriptor picks it per provider; an unmapped target is
	// empty, which is legible, where a guessed one would be wrong.
	Target     string
	Input      []byte
	Result     []byte
	Status     string
	DurationMS int
}

type SubagentEvent struct {
	ID        string
	AgentType string
}

// Interruption kinds. These are Crowbar concepts: a provider's own event names
// map onto them, and a provider missing one simply never reports it.
const (
	InterruptPermission   = "permission"
	InterruptNotification = "notification"
	InterruptElicitation  = "elicitation"
	InterruptCompaction   = "compaction"
)

// InterruptEvent is the agent being blocked on, or interrupted by, something
// outside the turn — a permission prompt, a notification, a compaction.
//
// These are the events whose absence caused every legibility failure observed in
// live testing: a provider blocked on a trust prompt showed nothing, and a
// compaction was invisible.
type InterruptEvent struct {
	Kind     string
	Detail   string
	Resolved bool
}
