package ledger

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// KindMove records a chat changing the folder its CLI runs in: asked for,
// then done, failed or cancelled. The chat owns this; the sidebar only draws it.
const KindMove Kind = "move"

// The phases of a move.
const (
	MoveRequested = "requested" // waits for the chat's turn boundary
	MoveDone      = "done"      // the chat runs in To from its next launch
	MoveFailed    = "failed"
	MoveCancelled = "cancelled"
)

// Move is the body of a move row. A request names where; done says what
// changed, which the next prompt's note and the write guard are built from.
type Move struct {
	Phase     string `json:"phase"`
	RequestID string `json:"requestId"`
	To        string `json:"to,omitempty"` // the workspace the chat runs in after
	ParentID  string `json:"parentId,omitempty"`
	Order     *int   `json:"order,omitempty"`
	Label     string `json:"label,omitempty"` // where, in the words the sidebar uses
	// BranchOut asks for a new branch cut from the current workspace's own
	// HEAD; Branch names it ("" generates one), and done names what was cut.
	BranchOut bool   `json:"branchOut,omitempty"`
	Branch    string `json:"branch,omitempty"`
	By        string `json:"by,omitempty"` // "user" or "agent"
	From      string `json:"from,omitempty"`
	FromDir   string `json:"fromDir,omitempty"`
	ToDir     string `json:"toDir,omitempty"`
	Base      string `json:"base,omitempty"` // the commit a branch-out starts from
	Repo      string `json:"repo,omitempty"` // set when the move crossed into another project
	Reason    string `json:"reason,omitempty"`
}

// ErrMoveWaits is a move's apply saying it cannot run yet (its parent thread
// has not moved); the move stays asked for.
var ErrMoveWaits = errors.New("ledger: move waits for its parent thread")

var movePhases = []string{MoveRequested, MoveDone, MoveFailed, MoveCancelled}

func checkMove(r Row) error {
	return checkNamed(r, func(b Move) bool {
		return b.RequestID != "" && slices.Contains(movePhases, b.Phase) &&
			(b.Phase != MoveRequested || b.To != "" || b.BranchOut)
	})
}

// moves is what the chat's move rows fold to.
type moves struct {
	pending *Move
	left    []string // folders the chat ran in and left, oldest first
	owed    *Move    // the newest move no delivered prompt has told the agent about
	fromDir string   // where the first move of the owed run started
	here    string   // the folder the last move entered
}

func (m moves) clone() moves {
	m.left = slices.Clone(m.left)
	return m
}

func (s *State) applyMove(r Row) {
	body, err := DecodeBody[Move](r)
	if err != nil {
		return
	}
	if body.Phase == MoveRequested {
		s.moves.pending = &body
		return
	}
	if s.moves.pending != nil && s.moves.pending.RequestID == body.RequestID {
		s.moves.pending = nil
	}
	if body.Phase != MoveDone {
		return
	}
	s.moves.here = body.ToDir
	s.moves.left = slices.DeleteFunc(s.moves.left, func(d string) bool { return d == body.ToDir })
	if body.FromDir != "" && body.FromDir != body.ToDir && !slices.Contains(s.moves.left, body.FromDir) {
		s.moves.left = append(s.moves.left, body.FromDir)
	}
	if s.moves.owed == nil {
		s.moves.fromDir = body.FromDir
	}
	s.moves.owed = &body
}

// noteDelivered: a turn opened on a Crowbar prompt carried the note.
func (s *State) noteDelivered(requestID string) {
	if requestID != "" {
		s.moves.owed = nil
	}
}

// PendingMove is the move asked for and not yet done, failed or cancelled.
func (s State) PendingMove() (Move, bool) {
	if s.moves.pending == nil {
		return Move{}, false
	}
	return *s.moves.pending, true
}

// MoveReady is the pending move once nothing in the chat still runs: a CLI
// is never moved mid-turn.
func (s State) MoveReady() (Move, bool) {
	m, ok := s.PendingMove()
	if !ok || s.Busy() || len(s.pending) > 0 {
		return Move{}, false
	}
	return m, true
}

// LeftDirs are the folders the chat ran in before and no longer owns.
func (s State) LeftDirs() []string { return slices.Clone(s.moves.left) }

// MovedTo is the folder the chat's last move entered, "" when it never moved.
func (s State) MovedTo() string { return s.moves.here }

// MoveNote is what the next prompt opens with after a move, "" when owed none.
func (s State) MoveNote() string {
	m := s.moves.owed
	if m == nil {
		return ""
	}
	var where strings.Builder
	fmt.Fprintf(&where, "You now work in %s", m.ToDir)
	switch {
	case m.Repo != "":
		fmt.Fprintf(&where, ", in repository %s", m.Repo)
		if m.Branch != "" {
			fmt.Fprintf(&where, " on branch %s", m.Branch)
		}
	case m.Branch != "":
		fmt.Fprintf(&where, ", on branch %s", m.Branch)
	}
	if m.Base != "" {
		fmt.Fprintf(&where, ", which starts from commit %s", m.Base)
	}
	from := s.moves.fromDir
	return fmt.Sprintf("<system-reminder>Crowbar moved this conversation to another folder. %s. "+
		"Run every command and write every file there. The folder you used before, %s, is no longer yours: "+
		"do not write to it. Uncommitted edits you made there were not brought over; do not re-apply them "+
		"unless the user asks.</system-reminder>\n\n", where.String(), from)
}
