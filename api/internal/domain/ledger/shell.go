package ledger

import "slices"

// KindShell records a command the main agent left running in the background:
// its start, then its end however it came about. It is not the chat's work, so
// a live shell keeps no chat busy.
const KindShell Kind = "shell"

// The phases of a shell row, and the words its end says.
const (
	ShellStart = "start"
	ShellEnd   = "end"

	ShellFinished = "finished"
	ShellFailed   = "failed"
	ShellStopped  = "stopped"
)

// ShellBody is the body of a shell row. A start names the command and the
// agent's own words for it; an end says how it ended.
type ShellBody struct {
	Phase   string `json:"phase"`
	ShellID string `json:"shellId"`
	Command string `json:"command,omitempty"`
	Label   string `json:"label,omitempty"`
	Status  string `json:"status,omitempty"`
}

// Shell is one background command of the chat; Status is "" while it runs.
type Shell struct {
	ID, Command, Label, Status string
	TurnID                     string
}

// Live reports whether the shell is still running.
func (s Shell) Live() bool { return s.Status == "" }

// ShellStatusEnds reports whether status is one a shell ends with.
func ShellStatusEnds(status string) bool {
	return slices.Contains([]string{ShellFinished, ShellFailed, ShellStopped}, status)
}

func checkShell(r Row) error {
	return checkNamed(r, func(b ShellBody) bool {
		if b.ShellID == "" {
			return false
		}
		return b.Phase == ShellStart || b.Phase == ShellEnd && ShellStatusEnds(b.Status)
	})
}

// ShellOf is the shell the chat knows by id.
func (s State) ShellOf(id string) (Shell, bool) {
	sh, ok := s.shells[id]
	return sh, ok
}

func (s *State) applyShell(r Row) {
	body, err := DecodeBody[ShellBody](r)
	if err != nil {
		return
	}
	known, seen := s.shells[body.ShellID]
	switch {
	case body.Phase == ShellStart && !seen:
		if s.shells == nil {
			s.shells = map[string]Shell{}
		}
		s.shells[body.ShellID] = Shell{ID: body.ShellID, Command: body.Command, Label: body.Label, TurnID: r.TurnID}
	case body.Phase == ShellEnd && seen && known.Live():
		known.Status = body.Status
		s.shells[body.ShellID] = known
	}
}
