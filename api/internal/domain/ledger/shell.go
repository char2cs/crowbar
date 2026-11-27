package ledger

import (
	"slices"
	"strings"
)

const KindShell Kind = "shell"

const (
	ShellStart    = "start"
	ShellNote     = "note"
	ShellEnd      = "end"
	ShellStopping = "stopping"

	ShellFinished = "finished"
	ShellFailed   = "failed"
	ShellStopped  = "stopped"

	ShellKindShell   = "shell"
	ShellKindMonitor = "monitor"

	ShellTailBytes = 2048
)

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
	Tail       string `json:"tail,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

type ShellEnding struct {
	Status, Tail, Summary string
	ExitCode              *int
}

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

type Shell struct {
	Status, TurnID string
	Stopping       bool
}

func (s Shell) Live() bool { return s.Status == "" }

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
		s.shells[body.ShellID] = Shell{TurnID: r.TurnID}
	case body.Phase == ShellStopping && seen && known.Live():
		known.Stopping = true
		s.shells[body.ShellID] = known
	case body.Phase == ShellEnd && seen && known.Live():
		known.Status = body.Status
		s.shells[body.ShellID] = known
	}
}
