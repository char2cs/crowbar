package ledger

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	ReasonStopped      = "stopped"
	ReasonProviderIdle = "provider_idle"
	ReasonSuperseded   = "superseded_by_prompt"
	ReasonTurnFailed   = "turn_failed"
)

const (
	PersistsTurnStart = "turn_start"
	PersistsTurnEnd   = "turn_end"
)

type TurnStatus string

const (
	TurnStatusPending     TurnStatus = "pending"
	TurnStatusRunning     TurnStatus = "running"
	TurnStatusDone        TurnStatus = "done"
	TurnStatusFailed      TurnStatus = "failed"
	TurnStatusInterrupted TurnStatus = "interrupted"
)

var turnEdges = map[TurnStatus][]TurnStatus{
	TurnStatusPending: {TurnStatusRunning},
	TurnStatusRunning: {TurnStatusDone, TurnStatusFailed, TurnStatusInterrupted},
}

func (s TurnStatus) Next(to TurnStatus) (TurnStatus, bool) {
	if slices.Contains(turnEdges[s], to) {
		return to, true
	}
	return s, false
}

type TurnEnded struct {
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	RunnerID       string `json:"runnerId,omitempty"`
	Effort         string `json:"effort,omitempty"`
	Diff           string `json:"diff,omitempty"`
	ProviderTurnID string `json:"providerTurnId,omitempty"`
}

type TurnFailed struct {
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
	Kind   string `json:"kind,omitempty"`
}

type Turn struct {
	ID, ProviderTurnID, RunnerID, RequestID string
	Status                                  TurnStatus
	StartedAt                               time.Time
}

func (t Turn) Names(providerTurnID string) bool {
	return providerTurnID == "" || t.ProviderTurnID == "" || providerTurnID == t.ProviderTurnID
}

type turnState struct {
	Turn
	provider, sessionID, persists string
	startSeq, closeSeq            int64
}

func (s State) RunningTurn() (Turn, bool) {
	if s.hasTurn && s.turn.Status == TurnStatusRunning {
		return s.turn.Turn, true
	}
	return Turn{}, false
}

func (s State) LastTurn() (Turn, bool) {
	return s.turn.Turn, s.hasTurn
}

func (s State) ReportedEffort() string { return s.effort }

func (s State) Busy() bool {
	_, ok := s.RunningTurn()
	return ok || len(s.Subagents()) > 0
}

func (s State) startsTurn(r Row) bool {
	return r.Scope != "" || !s.hasTurn || (s.turn.Status != TurnStatusRunning && s.turn.ID != r.TurnID)
}

func (s State) MovedTurn(r Row) bool {
	return s.hasTurn && r.Scope == "" && r.TurnID == s.turn.ID &&
		(r.Seq == s.turn.startSeq || r.Seq == s.turn.closeSeq)
}

func (s *State) applyTurnStarted(r Row, body TurnStarted) {
	if body.Persists != PersistsTurnEnd {
		s.markTurned(body.Provider, body.SessionID)
	}
	if r.Scope != "" {
		return
	}
	s.turn = turnState{
		Turn: Turn{
			ID: r.TurnID, ProviderTurnID: body.ProviderTurnID, RunnerID: body.RunnerID,
			RequestID: body.RequestID, Status: TurnStatusRunning, StartedAt: r.At,
		},
		provider: body.Provider, sessionID: body.SessionID, persists: body.Persists, startSeq: r.Seq,
	}
	s.hasTurn = true
	if gone := s.keys.openTurn(r.TurnID, body.ProviderTurnID, body.RunnerID); gone != "" {
		s.agents.forgetTurn(gone)
	}
}

func (s *State) applyTurnEnded(r Row) {
	body, err := DecodeBody[TurnEnded](r)
	if err != nil || r.Scope != "" {
		return
	}
	if !s.hasTurn || s.turn.ID != r.TurnID {
		return
	}
	to := TurnStatus(body.Status)
	next, ok := s.turn.Status.Next(to)
	if !ok || to == TurnStatusFailed {
		return
	}
	s.turn.Status, s.turn.closeSeq = next, r.Seq
	if body.ProviderTurnID != "" {
		s.keys.provider[body.ProviderTurnID] = r.TurnID
	}
	if body.Effort != "" {
		s.effort = body.Effort
	}
	outlived := s.SubagentRunning()
	s.abandonAsks(func(a PendingAsk) bool { return a.TurnID == r.TurnID && !a.OutlivesEnd(outlived) })
	if next == TurnStatusDone && s.turn.persists == PersistsTurnEnd {
		s.markTurned(s.turn.provider, s.turn.sessionID)
	}
}

func (s *State) applyTurnFailed(r Row) {
	if _, err := DecodeBody[TurnFailed](r); err != nil || r.Scope != "" || !s.hasTurn || s.turn.ID != r.TurnID {
		return
	}
	if next, ok := s.turn.Status.Next(TurnStatusFailed); ok {
		s.turn.Status, s.turn.closeSeq = next, r.Seq
		s.abandonAsks(func(a PendingAsk) bool { return a.TurnID == r.TurnID })
	}
}

func EndTurn(st State, status TurnStatus, reason string) ([]Row, error) {
	t, ok := st.RunningTurn()
	if !ok {
		return nil, nil
	}
	r, err := NewRow(KindTurnEnded, t.ID, "", TurnEnded{Status: string(status), Reason: reason, RunnerID: t.RunnerID})
	return []Row{r}, err
}

func EndTurnOf(st State, runnerID, providerTurnID string, end TurnEnded) (string, []Row, bool, error) {
	turnID, running, closed := st.turnOf(runnerID, providerTurnID)
	if closed {
		return turnID, nil, false, nil
	}
	end.RunnerID = runnerID
	r, err := NewRow(KindTurnEnded, turnID, "", end)
	return turnID, []Row{r}, running, err
}

func FailTurnOf(st State, runnerID, providerTurnID string, fail TurnFailed) (r Row, running, fresh bool, err error) {
	turnID, running, closed := st.turnOf(runnerID, providerTurnID)
	r, err = NewRow(KindTurnFailed, turnID, "", fail)
	return r, running, !running && !closed, err
}

func (s State) turnOf(runnerID, providerTurnID string) (id string, running, closed bool) {
	if s.hasTurn && s.turn.RunnerID == runnerID && s.turn.Names(providerTurnID) {
		running = s.turn.Status == TurnStatusRunning
		return s.turn.ID, running, !running
	}
	id, ok := s.ProviderTurn(providerTurnID)
	if providerTurnID == "" {
		id, ok = s.RunnerTurn(runnerID)
	}
	if ok && (id != s.turn.ID || s.turn.Status != TurnStatusRunning) {
		return id, false, true
	}
	return uuid.NewString(), false, false
}

func Stop(st State, diff string) ([]Row, error) {
	rows, err := Stopping(st)
	if err != nil {
		return nil, err
	}
	if t, ok := st.RunningTurn(); ok {
		ended, err := NewRow(KindTurnEnded, t.ID, "", TurnEnded{
			Status: string(TurnStatusInterrupted), Reason: ReasonStopped, RunnerID: t.RunnerID, Diff: diff,
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, ended)
	}
	settled, err := Settle(st, func(Prompt) bool { return true }, false, ReasonStopped)
	return append(rows, settled...), err
}

func Leave(st State, runnerID string, command func(text string) bool) ([]Row, error) {
	var rows []Row
	if t, ok := st.RunningTurn(); ok && t.RunnerID == runnerID {
		ended, err := EndTurn(st, TurnStatusInterrupted, MarkerSessionChanged)
		if err != nil {
			return nil, err
		}
		rows = ended
	}
	settled, err := SettleUntaken(st, func(p Prompt) bool { return p.RunnerID == runnerID }, command)
	return append(rows, settled...), err
}
