package models

type MoveKind string

const (
	MoveNoop MoveKind = "noop"

	MoveBind MoveKind = "bind"

	MoveToNew MoveKind = "move_new"

	MoveToKnown MoveKind = "move_known"
)

type Decision struct {
	Kind   MoveKind
	ChatID string
}
