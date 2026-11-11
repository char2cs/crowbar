package ledger

import (
	"slices"
	"strings"
)

// KindShell records a command the main agent left running in the background,
// or a monitor it set watching: its start, what it says while it runs, then its
// end however it came about. It is not the chat's work, so a live shell keeps
// no chat busy.
const KindShell Kind = "shell"

// The phases of a shell row, and the words its end says.
const (
	ShellStart = "start"
	ShellNote  = "note"
	ShellEnd   = "end"
	// ShellStopping says the agent was asked to end the shell; only its end
	// settles it.
	ShellStopping = "stopping"

	ShellFinished = "finished"
	ShellFailed   = "failed"
	ShellStopped  = "stopped"

	// A shell with no kind is a shell; a monitor is a command whose output lines
	// the agent is told of as they come.
	ShellKindShell   = "shell"
	ShellKindMonitor = "monitor"

	// ShellTailBytes bounds the output a shell row keeps: the end of it.
	ShellTailBytes = 2048
)

// ShellBody is the body of a shell row. A start names the command, the agent's
// own words for it and what the provider says of where and how it runs; a note
// is the newest thing it printed; an end says how it ended. Ref is the
// provider's handle on the process, which is not necessarily an OS pid.
type ShellBody struct {
	Phase      string `json:"phase"`
	ShellID    string `json:"shellId"`
	Kind       string `json:"kind,omitempty"`
	Command    string `json:"command,omitempty"`
	Label      string `json:"label,omitempty"`
	Cwd        string `json:"cwd,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Persistent bool   `json:"persistent,omitempty"`
	TimeoutMS  int    `json:"timeoutMs,omitempty"`
	Status     string `json:"status,omitempty"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	// Tail is the end of the shell's output (an end) or its newest line (a note).
	Tail string `json:"tail,omitempty"`
	// Summary is the provider's own one-line account of how the shell ended.
	Summary string `json:"summary,omitempty"`
}

// ShellEnding is what a shell's end says of it.
type ShellEnding struct {
	Status, Tail, Summary string
	ExitCode              *int
}

// ShellTail is the last ShellTailBytes of out, cut at a line start where one
// falls inside it.
func ShellTail(out string) string {
	out = strings.TrimRight(out, "\n")
	if len(out) <= ShellTailBytes {
		return out
	}
	out = out[len(out)-ShellTailBytes:]
	out = strings.ToValidUTF8(out, "")
	if i := strings.IndexByte(out, '\n'); i >= 0 && i < len(out)-1 {
		return out[i+1:]
	}
	return out
}

// Shell is one background command of the chat; Status is "" while it runs.
type Shell struct {
	ID, Command, Label, Status string
	TurnID                     string
	// Stopping is set once the agent was asked to end the shell, until it does.
	Stopping bool
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
		if b.Kind != "" && b.Kind != ShellKindShell && b.Kind != ShellKindMonitor {
			return false
		}
		return b.Phase == ShellStart || b.Phase == ShellStopping || b.Phase == ShellNote && b.Tail != "" ||
			b.Phase == ShellEnd && ShellStatusEnds(b.Status)
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
	case body.Phase == ShellStopping && seen && known.Live():
		known.Stopping = true
		s.shells[body.ShellID] = known
	case body.Phase == ShellEnd && seen && known.Live():
		known.Status = body.Status
		s.shells[body.ShellID] = known
	}
}
