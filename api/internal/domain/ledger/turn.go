package ledger

import (
	"slices"
	"time"
)

// Why a turn ended.
const (
	ReasonStopped      = "stopped"
	ReasonProviderIdle = "provider_idle"
	ReasonSuperseded   = "superseded_by_prompt"
	ReasonTurnFailed   = "turn_failed"
)

// When a provider's CLI has written a session it can resume: once a turn
// starts on it, or only once a turn on it completes.
const (
	PersistsTurnStart = "turn_start"
	PersistsTurnEnd   = "turn_end"
)

// TurnStatus is where a turn is in its life.
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

// Next reports the status a transition lands in and whether the turn diagram
// allows it: pending→running, running→done|failed|interrupted; nothing else.
func (s TurnStatus) Next(to TurnStatus) (TurnStatus, bool) {
	if slices.Contains(turnEdges[s], to) {
		return to, true
	}
	return s, false
}

// TurnEnded is the body of a turn_ended row: the turn is over, done or
// interrupted, and Reason says what closed it when the provider did not.
type TurnEnded struct {
	Status    string `json:"status"` // "done" | "interrupted"
	Reason    string `json:"reason,omitempty"`
	AsyncWork int    `json:"asyncWork,omitempty"` // deleted in T9
	Effort    string `json:"effort,omitempty"`
}

// TurnFailed is the body of a turn_failed row: the provider could not finish
// the turn.
type TurnFailed struct {
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// Turn is the chat's newest main-scope turn as the ledger has folded it.
type Turn struct {
	ID, ProviderTurnID, RunnerID, RequestID string
	Status                                  TurnStatus
	StartedAt                               time.Time
}

// Names reports whether an event naming providerTurnID is about t: only two
// ids that are both known and differ say it is about another turn.
func (t Turn) Names(providerTurnID string) bool {
	return providerTurnID == "" || t.ProviderTurnID == "" || providerTurnID == t.ProviderTurnID
}

// turnState is the newest turn plus what its end still decides.
type turnState struct {
	Turn
	provider, sessionID, persists string
	asyncWork                     int
}

// RunningTurn is the chat's main-scope turn while it runs.
func (s State) RunningTurn() (Turn, bool) {
	if s.hasTurn && s.turn.Status == TurnStatusRunning {
		return s.turn.Turn, true
	}
	return Turn{}, false
}

// Busy reports whether the chat is working: a main-scope turn runs, or the
// last one ended leaving background work its still-live runner carries (T9
// replaces that second clause with live subagents).
func (s State) Busy() bool {
	if _, ok := s.RunningTurn(); ok {
		return true
	}
	return s.hasTurn && s.turn.asyncWork > 0 && s.hasLive && s.live.RunnerID == s.turn.RunnerID
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
		provider: body.Provider, sessionID: body.SessionID, persists: body.Persists,
	}
	s.hasTurn = true
}

// applyTurnEnded closes the running turn it names; any other close is late.
func (s *State) applyTurnEnded(r Row) {
	body, err := DecodeBody[TurnEnded](r)
	if err != nil || r.Scope != "" || !s.hasTurn || s.turn.ID != r.TurnID {
		return
	}
	to := TurnStatus(body.Status)
	next, ok := s.turn.Status.Next(to)
	if !ok || to == TurnStatusFailed {
		return
	}
	s.turn.Status, s.turn.asyncWork = next, max(body.AsyncWork, 0)
	if next == TurnStatusDone && s.turn.persists == PersistsTurnEnd {
		s.markTurned(s.turn.provider, s.turn.sessionID)
	}
}

func (s *State) applyTurnFailed(r Row) {
	if _, err := DecodeBody[TurnFailed](r); err != nil || r.Scope != "" || !s.hasTurn || s.turn.ID != r.TurnID {
		return
	}
	s.turn.Status, _ = s.turn.Status.Next(TurnStatusFailed)
}

// EndTurn is the turn_ended row for the running turn; none when no turn runs.
func EndTurn(st State, status TurnStatus, reason string, asyncWork int, effort string) ([]Row, error) {
	t, ok := st.RunningTurn()
	if !ok {
		return nil, nil
	}
	r, err := NewRow(KindTurnEnded, t.ID, "", TurnEnded{
		Status: string(status), Reason: reason, AsyncWork: asyncWork, Effort: effort,
	})
	return []Row{r}, err
}

// FailTurn is the turn_failed row for the running turn; none when no turn runs.
func FailTurn(st State, reason, detail string) ([]Row, error) {
	t, ok := st.RunningTurn()
	if !ok {
		return nil, nil
	}
	r, err := NewRow(KindTurnFailed, t.ID, "", TurnFailed{Reason: reason, Detail: detail})
	return []Row{r}, err
}

// Stop is a user's Stop as rows: the running turn interrupted, and every
// pending prompt settled unconsumed, both for ReasonStopped.
func Stop(st State) ([]Row, error) {
	rows, err := EndTurn(st, TurnStatusInterrupted, ReasonStopped, 0, "")
	if err != nil {
		return nil, err
	}
	settled, err := Settle(st, func(Prompt) bool { return true }, false, ReasonStopped)
	return append(rows, settled...), err
}

// Leave is what runnerID leaves open when its CLI moves the chat onto another
// session: the turn it was running, interrupted, and the prompts it carried,
// unacknowledged. Nothing on this chat will close or acknowledge either.
func Leave(st State, runnerID string) ([]Row, error) {
	var rows []Row
	if t, ok := st.RunningTurn(); ok && t.RunnerID == runnerID {
		ended, err := EndTurn(st, TurnStatusInterrupted, MarkerSessionChanged, 0, "")
		if err != nil {
			return nil, err
		}
		rows = ended
	}
	settled, err := SettleCarried(st, runnerID, false, ReasonNotAcknowledged)
	return append(rows, settled...), err
}
