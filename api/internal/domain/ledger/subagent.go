package ledger

import (
	"cmp"
	"maps"
	"slices"
	"time"
)

const (
	SubagentStart  = "start"
	SubagentBound  = "bound"
	SubagentResume = "resume"
	SubagentResult = "result"
	SubagentStop   = "stop"
)

type SubagentStatus string

const (
	SubagentStarting    SubagentStatus = "starting"
	SubagentRunning     SubagentStatus = "running"
	SubagentDone        SubagentStatus = "done"
	SubagentFailed      SubagentStatus = "failed"
	SubagentInterrupted SubagentStatus = "interrupted"
	SubagentAbandoned   SubagentStatus = "abandoned"
)

var subagentEdges = map[SubagentStatus][]SubagentStatus{
	SubagentStarting:    {SubagentRunning, SubagentFailed, SubagentAbandoned, SubagentInterrupted},
	SubagentRunning:     {SubagentDone, SubagentFailed, SubagentInterrupted},
	SubagentDone:        {SubagentRunning},
	SubagentFailed:      {SubagentRunning},
	SubagentInterrupted: {SubagentRunning},
}

func (s SubagentStatus) Next(to SubagentStatus) (SubagentStatus, bool) {
	if slices.Contains(subagentEdges[s], to) {
		return to, true
	}
	return s, false
}

func (s SubagentStatus) Live() bool { return s == SubagentStarting || s == SubagentRunning }

type SubagentUsage struct {
	Tokens     int64 `json:"tokens,omitempty"`
	ToolUses   int64 `json:"toolUses,omitempty"`
	DurationMs int64 `json:"durationMs,omitempty"`
}

type SubagentBody struct {
	Phase       string         `json:"phase"`
	SpawnToolID string         `json:"spawnToolId,omitempty"`
	Type        string         `json:"type,omitempty"`
	Title       string         `json:"title,omitempty"`
	Status      string         `json:"status,omitempty"`
	Result      string         `json:"result,omitempty"`
	Usage       *SubagentUsage `json:"usage,omitempty"`
}

type Subagent struct {
	Scope, SpawnToolID, Type, Title, Result string
	Status                                  SubagentStatus
	TurnID                                  string
	StartedAt                               time.Time
	EndedAt                                 time.Time
	Usage                                   SubagentUsage
	Seq, EndSeq                             int64

	runnerID  string
	outlived  bool
	forgotten bool
}

type SubagentBook struct {
	agents  map[string]Subagent
	runners map[string]string
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

func (b SubagentBook) All() []Subagent {
	out := slices.Collect(maps.Values(b.agents))
	slices.SortFunc(out, func(a, c Subagent) int { return cmp.Compare(a.Seq, c.Seq) })
	return out
}

func (b SubagentBook) Of(scope string) (Subagent, bool) {
	a, ok := b.agents[scope]
	return a, ok && scope != ""
}

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
	case SubagentResume:
		b.resume(r)
	case SubagentResult, SubagentStop:
		b.finish(r, body)
	}
}

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

func (b *SubagentBook) resume(r Row) {
	a, ok := b.agents[r.Scope]
	if !ok {
		return
	}
	if next, moved := a.Status.Next(SubagentRunning); moved {
		a.Status, a.EndedAt, a.EndSeq, a.Result, a.outlived = next, time.Time{}, 0, "", false
		b.agents[r.Scope] = a
	}
}

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
	b.put(key, a)
}

func (b *SubagentBook) put(key string, a Subagent) {
	if a.forgotten && a.settled() {
		delete(b.agents, key)
		return
	}
	b.agents[key] = a
}

func (a Subagent) settled() bool {
	switch a.Status {
	case SubagentFailed, SubagentInterrupted, SubagentAbandoned:
		return true
	case SubagentDone:
		return a.Result != ""
	case SubagentStarting, SubagentRunning:
		return false
	}
	return false
}

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
		b.put(key, a)
	}
}

func (b *SubagentBook) interrupt(r Row, which func(Subagent) bool) {
	for key, a := range b.agents {
		if a.Status.Live() && which(a) {
			a.Status, a.EndedAt, a.EndSeq = SubagentInterrupted, r.At, r.Seq
			b.put(key, a)
		}
	}
}

func (b *SubagentBook) forgetTurn(turnID string) {
	for key, a := range b.agents {
		if a.TurnID != turnID {
			continue
		}
		a.forgotten = true
		b.put(key, a)
	}
	delete(b.runners, turnID)
}

func (s State) Subagents() []Subagent {
	return slices.DeleteFunc(s.agents.All(), func(a Subagent) bool { return !a.Status.Live() })
}

func (s State) SubagentRunning() bool {
	return slices.ContainsFunc(s.Subagents(), func(a Subagent) bool { return a.Status == SubagentRunning })
}

func (s State) SubagentOf(scope string) (Subagent, bool) { return s.agents.Of(scope) }

func (s State) SpawnedBy(toolID string) (Subagent, bool) { return s.agents.Spawned(toolID) }

func (s State) ScopeTurn(scope string) (string, bool) {
	a, ok := s.agents.Of(scope)
	return a.TurnID, ok
}

func (s State) ScopeOpen(scope string) bool {
	a, ok := s.agents.Of(scope)
	return ok && a.Status != SubagentDone && a.Status != SubagentFailed
}

func (s State) ScopeWaits(scope string) bool {
	a, ok := s.agents.Of(scope)
	return ok && a.Status == SubagentRunning && !a.outlived
}

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

func (s *State) applySubagent(r Row) {
	if body, err := DecodeBody[SubagentBody](r); err == nil && body.Phase == SubagentStop && r.Scope != "" {
		s.abandonAsks(func(a PendingAsk) bool { return a.Scope == r.Scope })
	}
}
