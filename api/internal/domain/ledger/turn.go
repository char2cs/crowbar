package ledger

import (
	"slices"
	"time"

	"github.com/google/uuid"
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
	Status   string `json:"status"` // "done" | "interrupted"
	Reason   string `json:"reason,omitempty"`
	RunnerID string `json:"runnerId,omitempty"` // whose end it is
	// AsyncWork is the background work the provider's own turn end restated;
	// nil on every other end, which says nothing of it. Deleted in T9.
	AsyncWork *int   `json:"asyncWork,omitempty"`
	Effort    string `json:"effort,omitempty"`
	// Diff is the provider's last whole diff of the turn's changes.
	Diff string `json:"diff,omitempty"`
}

// Async reports whether the end declared background work still running.
func (e TurnEnded) Async() bool { return e.AsyncWork != nil && *e.AsyncWork > 0 }

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

// turnState is the newest turn plus what its end still decides, and the seqs
// of the rows that started and closed it.
type turnState struct {
	Turn
	provider, sessionID, persists string
	startSeq, closeSeq            int64
}

// background is the background work a runner last restated. A Stop ends the
// runner's claim to any until it starts another turn.
type background struct {
	runnerID  string
	asyncWork int
	stopped   bool
}

// RunningTurn is the chat's main-scope turn while it runs.
func (s State) RunningTurn() (Turn, bool) {
	if s.hasTurn && s.turn.Status == TurnStatusRunning {
		return s.turn.Turn, true
	}
	return Turn{}, false
}

// LastTurn is the chat's newest main-scope turn, in any status.
func (s State) LastTurn() (Turn, bool) {
	return s.turn.Turn, s.hasTurn
}

// Busy reports whether the chat is working: a main-scope turn runs, or the
// live runner's last restatement, late or not, left background work (T9
// replaces that second clause with live subagents).
func (s State) Busy() bool {
	if _, ok := s.RunningTurn(); ok {
		return true
	}
	return s.bg.asyncWork > 0 && s.hasLive && s.live.RunnerID == s.bg.runnerID
}

// startsTurn reports whether a turn_started may open its turn: never over a
// running turn, and never reopening a closed one.
func (s State) startsTurn(r Row) bool {
	return r.Scope != "" || !s.hasTurn || (s.turn.Status != TurnStatusRunning && s.turn.ID != r.TurnID)
}

// MovedTurn reports whether r started or closed the chat's newest turn; a
// late row moved nothing.
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
	s.keys.openTurn(r.TurnID, body.ProviderTurnID, body.RunnerID)
	if s.bg.runnerID == body.RunnerID {
		s.bg.stopped = false
	}
}

// applyTurnEnded closes the running turn it names; any other close is late.
// Either way a Stop ends its runner's claim to background work, and a
// provider's own end restates it.
func (s *State) applyTurnEnded(r Row) {
	body, err := DecodeBody[TurnEnded](r)
	if err != nil || r.Scope != "" {
		return
	}
	switch {
	case body.Reason == ReasonStopped && s.hasLive && body.RunnerID == s.live.RunnerID:
		s.bg = background{runnerID: body.RunnerID, stopped: true}
	case body.AsyncWork != nil:
		s.restate(body.RunnerID, *body.AsyncWork)
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
	s.abandonAsks(func(a PendingAsk) bool { return a.TurnID == r.TurnID && !a.OutlivesEnd(body.Async()) })
	if next == TurnStatusDone && s.turn.persists == PersistsTurnEnd {
		s.markTurned(s.turn.provider, s.turn.sessionID)
	}
}

// applyTurnFailed fails the running turn it names. A failed turn restates no
// background work, so a failure never leaves the chat refusing prompts on it.
func (s *State) applyTurnFailed(r Row) {
	if _, err := DecodeBody[TurnFailed](r); err != nil || r.Scope != "" || !s.hasTurn || s.turn.ID != r.TurnID {
		return
	}
	s.restate(s.turn.RunnerID, 0)
	if next, ok := s.turn.Status.Next(TurnStatusFailed); ok {
		s.turn.Status, s.turn.closeSeq = next, r.Seq
		s.abandonAsks(func(a PendingAsk) bool { return a.TurnID == r.TurnID })
	}
}

// restate records runnerID's background work, when it is the live runner and
// no Stop ended its claim.
func (s *State) restate(runnerID string, asyncWork int) {
	if !s.hasLive || s.live.RunnerID != runnerID || (s.bg.stopped && s.bg.runnerID == runnerID) {
		return
	}
	s.bg = background{runnerID: runnerID, asyncWork: max(asyncWork, 0)}
}

// EndTurn is the turn_ended row for the running turn; none when no turn runs.
func EndTurn(st State, status TurnStatus, reason string) ([]Row, error) {
	t, ok := st.RunningTurn()
	if !ok {
		return nil, nil
	}
	r, err := NewRow(KindTurnEnded, t.ID, "", TurnEnded{Status: string(status), Reason: reason, RunnerID: t.RunnerID})
	return []Row{r}, err
}

// EndTurnOf is runnerID's report that the turn providerTurnID names ended. It
// closes the running turn when runnerID runs it and the name fits; otherwise
// the row is late and names the closed turn it is about, or a fresh id.
func EndTurnOf(st State, runnerID, providerTurnID string, end TurnEnded) (Row, bool, error) {
	turnID, closes := st.turnOf(runnerID, providerTurnID)
	end.RunnerID = runnerID
	r, err := NewRow(KindTurnEnded, turnID, "", end)
	return r, closes, err
}

// FailTurnOf is EndTurnOf for runnerID's report that the turn failed.
func FailTurnOf(st State, runnerID, providerTurnID string, fail TurnFailed) (Row, bool, error) {
	turnID, closes := st.turnOf(runnerID, providerTurnID)
	r, err := NewRow(KindTurnFailed, turnID, "", fail)
	return r, closes, err
}

// turnOf is the turn a report by runnerID naming providerTurnID is about, and
// whether it is running. A late report is about the kept turn it names, else
// its runner's newest; a fresh id when the ledger holds no such turn.
func (s State) turnOf(runnerID, providerTurnID string) (string, bool) {
	if s.hasTurn && s.turn.RunnerID == runnerID && s.turn.Names(providerTurnID) {
		return s.turn.ID, s.turn.Status == TurnStatusRunning
	}
	id, ok := s.ProviderTurn(providerTurnID)
	if providerTurnID == "" {
		id, ok = s.RunnerTurn(runnerID)
	}
	if ok && (id != s.turn.ID || s.turn.Status != TurnStatusRunning) {
		return id, false
	}
	return uuid.NewString(), false
}

// Stop is a user's Stop as rows: the running turn interrupted, and every
// pending prompt settled unconsumed, both for ReasonStopped.
func Stop(st State) ([]Row, error) {
	rows, err := EndTurn(st, TurnStatusInterrupted, ReasonStopped)
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
		ended, err := EndTurn(st, TurnStatusInterrupted, MarkerSessionChanged)
		if err != nil {
			return nil, err
		}
		rows = ended
	}
	settled, err := SettleCarried(st, runnerID, false, ReasonNotAcknowledged)
	return append(rows, settled...), err
}
