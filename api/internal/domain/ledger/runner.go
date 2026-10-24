package ledger

import "time"

// RunnerStarted is the body of a runner_started row: a CLI was launched on
// the chat, with what Crowbar rendered into its argv.
type RunnerStarted struct {
	RunnerID              string `json:"runnerId"`
	Provider              string `json:"provider"`
	TerminalSession       string `json:"terminalSession"`
	LaunchModel           string `json:"launchModel"`
	LaunchEffort          string `json:"launchEffort"`
	LaunchPermissionLevel string `json:"launchPermissionLevel"`
	ResumeSessionID       string `json:"resumeSessionId"`
}

// RunnerExited is the body of a runner_exited row: the runner's process is
// gone, and Reason (a domain.AgentExit* value) says why.
type RunnerExited struct {
	RunnerID string `json:"runnerId"`
	Reason   string `json:"reason"`
	ExitCode int    `json:"exitCode"`
}

// RunnerExit is how the chat's last runner ended.
type RunnerExit struct {
	RunnerID, Reason string
	ExitCode         int
	At               time.Time
}

// LiveRunner is the runner started on the chat that has not exited.
func (s State) LiveRunner() (RunnerStarted, bool) {
	return s.live, s.hasLive
}

// LastExit is the newest runner_exited the chat holds.
func (s State) LastExit() (RunnerExit, bool) {
	return s.lastExit, s.hasExit
}

func (s *State) applyRunnerStarted(r Row) {
	if body, err := DecodeBody[RunnerStarted](r); err == nil {
		s.live, s.hasLive = body, true
	}
}

// applyRunnerExited ends the live runner only when it is the one named: an
// exit of an earlier runner is late.
func (s *State) applyRunnerExited(r Row) {
	body, err := DecodeBody[RunnerExited](r)
	if err != nil {
		return
	}
	if s.hasLive && s.live.RunnerID == body.RunnerID {
		s.live, s.hasLive = RunnerStarted{}, false
	}
	s.lastExit = RunnerExit{RunnerID: body.RunnerID, Reason: body.Reason, ExitCode: body.ExitCode, At: r.At}
	s.hasExit = true
	// What the gone process was asking or running, nothing will answer or close.
	s.abandonAsks(func(PendingAsk) bool { return true })
	for id, sc := range s.keys.scopes {
		sc.stopped = true
		s.keys.scopes[id] = sc
	}
}

// Exit is a runner's end as rows: the turn it was running interrupted, its
// runner_exited, then a settle of every prompt it still carried, both for
// reason.
func Exit(st State, e RunnerExited, reason string) ([]Row, error) {
	var rows []Row
	if t, ok := st.RunningTurn(); ok && t.RunnerID == e.RunnerID {
		ended, err := EndTurn(st, TurnStatusInterrupted, reason)
		if err != nil {
			return nil, err
		}
		rows = ended
	}
	row, err := NewRow(KindRunnerExited, "", "", e)
	if err != nil {
		return nil, err
	}
	settled, err := SettleCarried(st, e.RunnerID, false, reason)
	if err != nil {
		return nil, err
	}
	return append(append(rows, row), settled...), nil
}
