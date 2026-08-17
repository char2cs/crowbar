package models

// MoveKind names the outcome the context-move reducer reaches for one hook.
type MoveKind string

const (
	// MoveNoop means the announced conversation id matches the one already
	// running under this runner — nothing happened.
	MoveNoop MoveKind = "noop"
	// MoveBind means this is the runner's FIRST announced conversation id. It
	// stays where it is, even if the id is already known (a resumed spawn).
	MoveBind MoveKind = "bind"
	// MoveToNew means the runner moved to a conversation id nobody has seen, so a
	// new chat must be minted for it.
	MoveToNew MoveKind = "move_new"
	// MoveToKnown means the runner moved to a conversation id that already has a
	// chat, so the runner should be re-pointed at that chat.
	MoveToKnown MoveKind = "move_known"
)

// Decision is the reducer's result: what kind of move happened, and — only for
// MoveToKnown — which chat it moved to.
type Decision struct {
	Kind   MoveKind
	ChatID string
}
