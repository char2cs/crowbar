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
}

// Exit is a runner's end as rows: its runner_exited, then a settle, for
// settleReason, of every prompt it still carried.
func Exit(st State, e RunnerExited, settleReason string) ([]Row, error) {
	row, err := NewRow(KindRunnerExited, "", "", e)
	if err != nil {
		return nil, err
	}
	settled, err := SettleCarried(st, e.RunnerID, false, settleReason)
	if err != nil {
		return nil, err
	}
	return append([]Row{row}, settled...), nil
}
