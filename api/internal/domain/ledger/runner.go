package ledger

import "time"

type RunnerStarted struct {
	RunnerID              string `json:"runnerId"`
	Provider              string `json:"provider"`
	TerminalSession       string `json:"terminalSession"`
	LaunchModel           string `json:"launchModel"`
	LaunchEffort          string `json:"launchEffort"`
	LaunchPermissionLevel string `json:"launchPermissionLevel"`
	ResumeSessionID       string `json:"resumeSessionId"`
}

type RunnerExited struct {
	RunnerID string `json:"runnerId"`
	Reason   string `json:"reason"`
	ExitCode int    `json:"exitCode"`
	Detail   string `json:"detail,omitempty"`
}

type RunnerExit struct {
	RunnerID, Reason string
	ExitCode         int
	At               time.Time
}

func (s State) LiveRunner() (RunnerStarted, bool) {
	return s.live, s.hasLive
}

func (s State) LastExit() (RunnerExit, bool) {
	return s.lastExit, s.hasExit
}

func (s State) LastLaunch(provider string) RunnerStarted { return s.launches[provider] }

func (s *State) applyRunnerStarted(r Row) {
	if body, err := DecodeBody[RunnerStarted](r); err == nil {
		s.live, s.hasLive = body, true
		if s.launches == nil {
			s.launches = map[string]RunnerStarted{}
		}
		s.launches[body.Provider] = body
	}
}

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
	ran := func(runnerID string) bool { return runnerID == "" || runnerID == body.RunnerID }
	s.abandonAsks(func(a PendingAsk) bool { return ran(s.keys.runnerOf(a.TurnID)) })
	for id, sh := range s.shells {
		if sh.Live() && ran(s.keys.runnerOf(sh.TurnID)) {
			sh.Status = ShellStopped
			s.shells[id] = sh
		}
	}
}

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
