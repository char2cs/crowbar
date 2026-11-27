package ledger

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const KindMove Kind = "move"

const (
	MoveRequested = "requested"
	MoveDone      = "done"
	MoveFailed    = "failed"
	MoveCancelled = "cancelled"
)

type Move struct {
	Phase     string `json:"phase"`
	RequestID string `json:"requestId"`
	To        string `json:"to,omitempty"`
	ParentID  string `json:"parentId,omitempty"`
	Order     *int   `json:"order,omitempty"`
	Label     string `json:"label,omitempty"`
	BranchOut bool   `json:"branchOut,omitempty"`
	Branch    string `json:"branch,omitempty"`
	By        string `json:"by,omitempty"`
	From      string `json:"from,omitempty"`
	FromDir   string `json:"fromDir,omitempty"`
	ToDir     string `json:"toDir,omitempty"`
	Base      string `json:"base,omitempty"`
	Repo      string `json:"repo,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

var ErrMoveWaits = errors.New("ledger: move waits for its parent thread")

var movePhases = []string{MoveRequested, MoveDone, MoveFailed, MoveCancelled}

func checkMove(r Row) error {
	return checkNamed(r, func(b Move) bool {
		return b.RequestID != "" && slices.Contains(movePhases, b.Phase) &&
			(b.Phase != MoveRequested || b.To != "" || b.BranchOut)
	})
}

type moves struct {
	pending *Move
	left    []string
	owed    *Move
	fromDir string
	here    string
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

func (s *State) noteDelivered(requestID string) {
	if requestID != "" {
		s.moves.owed = nil
	}
}

func (s State) PendingMove() (Move, bool) {
	if s.moves.pending == nil {
		return Move{}, false
	}
	return *s.moves.pending, true
}

func (s State) MoveReady() (Move, bool) {
	m, ok := s.PendingMove()
	if !ok || s.Busy() || len(s.pending) > 0 {
		return Move{}, false
	}
	return m, true
}

func (s State) LeftDirs() []string { return slices.Clone(s.moves.left) }

func (s State) MovedTo() string { return s.moves.here }

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
	rename := ""
	if m.BranchOut {
		rename = " This branch is yours: if its name does not say what the work is, rename it with " +
			"set_branch_name to a short descriptive one."
	}
	from := s.moves.fromDir
	return fmt.Sprintf("<system-reminder>Crowbar moved this conversation to another folder. %s. "+
		"Run every command and write every file there. The folder you used before, %s, is no longer yours: "+
		"do not write to it. Uncommitted edits you made there were not brought over; do not re-apply them "+
		"unless the user asks.%s</system-reminder>\n\n", where.String(), from, rename)
}
