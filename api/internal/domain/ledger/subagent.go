package ledger

import (
	"cmp"
	"maps"
	"slices"
	"time"
)

// The phases of a subagent row.
const (
	SubagentStart  = "start"  // a spawning call opened it, or it announced itself
	SubagentBound  = "bound"  // its spawning call is known to run as Scope
	SubagentResult = "result" // it handed its result back
	SubagentStop   = "stop"   // it ended
)

// SubagentStatus is where a subagent is in its life.
type SubagentStatus string

const (
	SubagentStarting    SubagentStatus = "starting"
	SubagentRunning     SubagentStatus = "running"
	SubagentDone        SubagentStatus = "done"
	SubagentFailed      SubagentStatus = "failed"
	SubagentInterrupted SubagentStatus = "interrupted"
	SubagentAbandoned   SubagentStatus = "abandoned"
)

// A Stop or a runner's exit interrupts a subagent that has not bound yet too.
var subagentEdges = map[SubagentStatus][]SubagentStatus{
	SubagentStarting: {SubagentRunning, SubagentFailed, SubagentAbandoned, SubagentInterrupted},
	SubagentRunning:  {SubagentDone, SubagentFailed, SubagentInterrupted},
}

// Next reports the status a transition lands in and whether the lifecycle
// allows it: starting→running|failed|abandoned|interrupted,
// running→done|failed|interrupted; nothing else.
func (s SubagentStatus) Next(to SubagentStatus) (SubagentStatus, bool) {
	if slices.Contains(subagentEdges[s], to) {
		return to, true
	}
	return s, false
}

// Live reports whether the subagent may still work: starting or running.
func (s SubagentStatus) Live() bool { return s == SubagentStarting || s == SubagentRunning }

// SubagentUsage is what a subagent spent, as its provider reported it.
type SubagentUsage struct {
	Tokens     int64 `json:"tokens,omitempty"`
	ToolUses   int64 `json:"toolUses,omitempty"`
	DurationMs int64 `json:"durationMs,omitempty"`
}

// SubagentBody is the body of a subagent row. Its row's Scope is the
// subagent's, "" while only its spawning call (SpawnToolID) names it.
type SubagentBody struct {
	Phase       string         `json:"phase"`
	SpawnToolID string         `json:"spawnToolId,omitempty"`
	Type        string         `json:"type,omitempty"`
	Title       string         `json:"title,omitempty"`
	Status      string         `json:"status,omitempty"` // a result's or a stop's: failed, interrupted, else done
	Result      string         `json:"result,omitempty"`
	Usage       *SubagentUsage `json:"usage,omitempty"`
}

// Subagent is one subagent as its rows left it: bound to the call that
// spawned it, and living past the turn that did.
type Subagent struct {
	Scope, SpawnToolID, Type, Title, Result string
	Status                                  SubagentStatus
	TurnID                                  string // the turn that spawned it; it may outlive that turn
	StartedAt                               time.Time
	EndedAt                                 time.Time
	Usage                                   SubagentUsage
	Seq, EndSeq                             int64 // its first row's, and the one that ended it

	runnerID string
	// outlived: a main-scope turn end came after it started running.
	outlived bool
}

// SubagentBook folds the subagent rows of a chat into its subagents. It also
// reads the turn and runner rows their lifecycle turns on.
type SubagentBook struct {
	agents  map[string]Subagent // by Scope, or by "\x00"+SpawnToolID until bound
	runners map[string]string   // turn → the runner that ran it
}

func spawnKey(toolID string) string { return "\x00" + toolID }

func (b *SubagentBook) init() {
	if b.agents == nil {
		b.agents, b.runners = map[string]Subagent{}, map[string]string{}
	}
}

func (b SubagentBook) clone() SubagentBook {
	return SubagentBook{agents: maps.Clone(b.agents), runners: maps.Clone(b.runners)}
}

// All are the chat's subagents in the order they began.
func (b SubagentBook) All() []Subagent {
	out := slices.Collect(maps.Values(b.agents))
	slices.SortFunc(out, func(a, c Subagent) int { return cmp.Compare(a.Seq, c.Seq) })
	return out
}

// Of is the subagent bound as scope.
func (b SubagentBook) Of(scope string) (Subagent, bool) {
	a, ok := b.agents[scope]
	return a, ok && scope != ""
}

// Spawned is the subagent the tool call toolID spawned, bound or not.
func (b SubagentBook) Spawned(toolID string) (Subagent, bool) {
	if a, ok := b.agents[spawnKey(toolID)]; ok {
		return a, true
	}
	for _, a := range b.agents {
		if a.SpawnToolID == toolID && toolID != "" {
			return a, true
		}
	}
	return Subagent{}, false
}

// Apply folds r; a row of another kind, or one the lifecycle does not allow,
// changes nothing.
func (b *SubagentBook) Apply(r Row) {
	if apply, ok := bookAppliers[r.Kind]; ok {
		b.init()
		apply(b, r)
	}
}

var bookAppliers = map[Kind]func(*SubagentBook, Row){
	KindTurnStarted: func(b *SubagentBook, r Row) {
		if body, err := DecodeBody[TurnStarted](r); err == nil && r.Scope == "" {
			b.runners[r.TurnID] = body.RunnerID
		}
	},
	KindTurnEnded:  (*SubagentBook).endTurn,
	KindTurnFailed: (*SubagentBook).endTurn,
	KindRunnerExited: func(b *SubagentBook, r Row) {
		if body, err := DecodeBody[RunnerExited](r); err == nil {
			b.interrupt(r, func(a Subagent) bool { return a.runnerID == "" || a.runnerID == body.RunnerID })
		}
	},
	KindSubagent: func(b *SubagentBook, r Row) {
		if body, err := DecodeBody[SubagentBody](r); err == nil {
			b.applyRow(r, body)
		}
	},
}

func (b *SubagentBook) applyRow(r Row, body SubagentBody) {
	switch body.Phase {
	case SubagentStart:
		b.start(r, body)
	case SubagentBound:
		b.bind(r, body)
	case SubagentResult, SubagentStop:
		b.finish(r, body)
	}
}

// start opens a subagent: starting under its spawning call, or running as its
// scope when the row names one (a start that also binds).
func (b *SubagentBook) start(r Row, body SubagentBody) {
	if r.Scope == "" {
		if _, seen := b.Spawned(body.SpawnToolID); seen || body.SpawnToolID == "" {
			return
		}
		b.agents[spawnKey(body.SpawnToolID)] = b.newAgent(r, body, SubagentStarting)
		return
	}
	if a, seen := b.agents[r.Scope]; seen {
		a.Type, a.Title = cmp.Or(a.Type, body.Type), cmp.Or(a.Title, body.Title)
		b.agents[r.Scope] = a
		return
	}
	b.agents[r.Scope] = b.newAgent(r, body, SubagentRunning)
	if body.SpawnToolID != "" {
		b.bind(r, body)
	}
}

func (b *SubagentBook) newAgent(r Row, body SubagentBody, status SubagentStatus) Subagent {
	return Subagent{
		Scope: r.Scope, SpawnToolID: body.SpawnToolID, Type: body.Type, Title: body.Title, Status: status,
		TurnID: r.TurnID, StartedAt: r.At, Seq: r.Seq, runnerID: b.runners[r.TurnID],
	}
}

// bind moves the subagent spawnToolID started onto its scope. One that
// announced itself first keeps its own record, and takes the call's.
func (b *SubagentBook) bind(r Row, body SubagentBody) {
	if r.Scope == "" || body.SpawnToolID == "" {
		return
	}
	starting, hadStart := b.agents[spawnKey(body.SpawnToolID)]
	delete(b.agents, spawnKey(body.SpawnToolID))
	a, announced := b.agents[r.Scope]
	switch {
	case announced:
		a.SpawnToolID = cmp.Or(a.SpawnToolID, body.SpawnToolID)
		a.Title, a.Type = cmp.Or(a.Title, starting.Title), cmp.Or(a.Type, starting.Type)
		if hadStart && starting.Seq < a.Seq {
			a.Seq, a.StartedAt, a.TurnID, a.runnerID = starting.Seq, starting.StartedAt, starting.TurnID, starting.runnerID
		}
	case hadStart:
		a = starting
		a.Scope = r.Scope
		a.Status, _ = a.Status.Next(SubagentRunning)
	default:
		a = b.newAgent(r, body, SubagentRunning)
	}
	a.Type = cmp.Or(a.Type, body.Type)
	b.agents[r.Scope] = a
}

// finish ends a subagent with what it reported; a later report of one already
// over only fills in what the earlier left out.
func (b *SubagentBook) finish(r Row, body SubagentBody) {
	key := r.Scope
	if key == "" {
		key = spawnKey(body.SpawnToolID)
	}
	a, ok := b.agents[key]
	if !ok {
		return
	}
	to := SubagentDone
	if s := SubagentStatus(body.Status); s == SubagentFailed || s == SubagentInterrupted {
		to = s
	}
	if next, moved := a.Status.Next(to); moved {
		a.Status, a.EndedAt, a.EndSeq = next, r.At, r.Seq
	}
	a.Result = cmp.Or(a.Result, body.Result)
	if body.Usage != nil && a.Usage == (SubagentUsage{}) {
		a.Usage = *body.Usage
	}
	b.agents[key] = a
}

// endTurn abandons what the ending turn started and never bound, and marks
// what runs as outliving the end.
func (b *SubagentBook) endTurn(r Row) {
	if r.Scope != "" {
		return
	}
	for key, a := range b.agents {
		switch {
		case a.Status == SubagentStarting && a.TurnID == r.TurnID:
			a.Status, a.EndedAt, a.EndSeq = SubagentAbandoned, r.At, r.Seq
		case a.Status == SubagentRunning:
			a.outlived = true
		default:
			continue
		}
		b.agents[key] = a
	}
}

func (b *SubagentBook) interrupt(r Row, which func(Subagent) bool) {
	for key, a := range b.agents {
		if a.Status.Live() && which(a) {
			a.Status, a.EndedAt, a.EndSeq = SubagentInterrupted, r.At, r.Seq
			b.agents[key] = a
		}
	}
}

// forgetTurn drops what turnID left that is over: nothing will name it again.
func (b *SubagentBook) forgetTurn(turnID string) {
	maps.DeleteFunc(b.agents, func(_ string, a Subagent) bool { return a.TurnID == turnID && !a.Status.Live() })
	delete(b.runners, turnID)
}

// Subagents are the chat's subagents still starting or running, by their start.
func (s State) Subagents() []Subagent {
	return slices.DeleteFunc(s.agents.All(), func(a Subagent) bool { return !a.Status.Live() })
}

// SubagentRunning reports whether a bound subagent of the chat still runs.
func (s State) SubagentRunning() bool {
	return slices.ContainsFunc(s.Subagents(), func(a Subagent) bool { return a.Status == SubagentRunning })
}

// SubagentOf is the subagent bound as scope, in any status.
func (s State) SubagentOf(scope string) (Subagent, bool) { return s.agents.Of(scope) }

// SpawnedBy is the subagent the tool call toolID spawned, bound or not.
func (s State) SpawnedBy(toolID string) (Subagent, bool) { return s.agents.Spawned(toolID) }

// ScopeTurn is the turn that spawned scope.
func (s State) ScopeTurn(scope string) (string, bool) {
	a, ok := s.agents.Of(scope)
	return a.TurnID, ok
}

// ScopeOpen reports whether scope is bound and has not ended on its own
// report: an interrupted one's late reports still reach it.
func (s State) ScopeOpen(scope string) bool {
	a, ok := s.agents.Of(scope)
	return ok && a.Status != SubagentDone && a.Status != SubagentFailed
}

// ScopeWaits reports whether scope runs and no turn end came after it began
// running: what it asks is waited on.
func (s State) ScopeWaits(scope string) bool {
	a, ok := s.agents.Of(scope)
	return ok && a.Status == SubagentRunning && !a.outlived
}

// Stopping is what a user's Stop records of the chat's live subagents: each
// one interrupted.
func Stopping(st State) ([]Row, error) {
	var rows []Row
	for _, a := range st.Subagents() {
		r, err := NewRow(KindSubagent, a.TurnID, a.Scope, SubagentBody{
			Phase: SubagentStop, SpawnToolID: a.SpawnToolID, Status: string(SubagentInterrupted),
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// applySubagent closes what a stopped subagent was asking: nothing will answer.
func (s *State) applySubagent(r Row) {
	if body, err := DecodeBody[SubagentBody](r); err == nil && body.Phase == SubagentStop && r.Scope != "" {
		s.abandonAsks(func(a PendingAsk) bool { return a.Scope == r.Scope })
	}
}
