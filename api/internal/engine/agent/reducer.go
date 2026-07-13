package agent

// MoveKind names the outcome Decide reaches for a single hook event.
type MoveKind string

const (
	// MoveNoop means the announced conversation id matches the one already
	// running under this runner — nothing happened.
	MoveNoop MoveKind = "noop"
	// MoveBind means this is the runner's FIRST announced conversation id.
	// It stays where it is, even if the id happens to be known already (a
	// resumed spawn).
	MoveBind MoveKind = "bind"
	// MoveToNew means the runner moved to a conversation id nobody has seen
	// before, so a new chat must be minted for it.
	MoveToNew MoveKind = "move_new"
	// MoveToKnown means the runner moved to a conversation id that already
	// has a chat, so the runner should be re-pointed at that chat.
	MoveToKnown MoveKind = "move_known"
)

// Decision is the result of Decide: what kind of move happened, and — only
// for MoveToKnown — which chat it moved to.
type Decision struct {
	Kind   MoveKind
	ChatID string
}

// Decide is the context-move reducer. It is a PURE function of exactly two
// facts and nothing else:
//
//  1. did the conversation id under this runner change?
//  2. is the new id one we already know?
//
// It deliberately takes NO `source` argument, and must never be changed to
// read one. Claude reports source=clear where Codex reports source=startup
// for the very same event (verified against the real binaries, spec §7), so
// any branch on that vocabulary is provider-specific and will break on the
// next CLI. Branching only on the two facts above is what makes this engine
// provider-agnostic.
//
// Decide also never decides whether a move is ALLOWED — there is no reject
// outcome. By the time a hook fires, the CLI has already switched
// conversation; Crowbar cannot refuse it and cannot push the CLI back. Decide
// reconciles a fait accompli, it does not authorise one (spec §3).
func Decide(
	currentSession string,
	announcedSession string,
	knownChatID string,
	known bool,
) Decision {
	switch {
	case announcedSession == currentSession:
		return Decision{Kind: MoveNoop}
	case currentSession == "":
		return Decision{Kind: MoveBind}
	case known:
		return Decision{Kind: MoveToKnown, ChatID: knownChatID}
	default:
		return Decision{Kind: MoveToNew}
	}
}
