package ledger

// The phases of a subagent.
const (
	SubagentStart = "start"
	SubagentStop  = "stop"
)

// Subagent is the body of a subagent row; the row's Scope is the subagent.
type Subagent struct {
	Phase string `json:"phase"`
	Type  string `json:"type,omitempty"`
}

// scopeKeys is a subagent scope as its rows left it. Outlived: a main-scope
// turn end came after it opened. Exited: its runner is gone, its stop may not.
type scopeKeys struct {
	turnID, runnerID                   string
	started, stopped, outlived, exited bool
}

// ScopeTurn is the turn that spawned scope.
func (s State) ScopeTurn(scope string) (string, bool) {
	sc, ok := s.keys.scopes[scope]
	return sc.turnID, ok
}

// ScopeOpen reports whether scope started and has not stopped.
func (s State) ScopeOpen(scope string) bool {
	sc := s.keys.scopes[scope]
	return sc.started && !sc.stopped
}

// ScopeWaits reports whether scope is open, its runner lives, and no turn end
// came after it opened: what it asks is waited on.
func (s State) ScopeWaits(scope string) bool {
	sc := s.keys.scopes[scope]
	return sc.started && !sc.stopped && !sc.outlived && !sc.exited
}

// HasScopePhase reports whether scope's phase is already recorded.
func (s State) HasScopePhase(scope, phase string) bool {
	sc, ok := s.keys.scopes[scope]
	if !ok {
		return false
	}
	if phase == SubagentStart {
		return sc.started
	}
	return sc.stopped
}

func (s *State) applySubagent(r Row) {
	body, err := DecodeBody[Subagent](r)
	if err != nil || r.Scope == "" {
		return
	}
	k := s.keys.init()
	sc, ok := k.scopes[r.Scope]
	if !ok {
		sc.turnID, sc.runnerID = r.TurnID, k.runnerOf(r.TurnID)
	}
	switch body.Phase {
	case SubagentStart:
		sc.started = true
	case SubagentStop:
		sc.stopped = true
		s.abandonAsks(func(a PendingAsk) bool { return a.Scope == r.Scope })
	}
	k.scopes[r.Scope] = sc
}

// outliveScopes marks every open scope a main-scope turn end r came after.
func (s *State) outliveScopes(r Row) {
	if r.Scope != "" {
		return
	}
	for id, sc := range s.keys.scopes {
		if sc.started && !sc.stopped {
			sc.outlived = true
			s.keys.scopes[id] = sc
		}
	}
}

// spawnScope opens scope, spawned by a tool call of turnID, unless it was ever
// recorded: a stopped one is never reopened.
func (s *State) spawnScope(scope, turnID string) {
	k := s.keys.init()
	if _, ok := k.scopes[scope]; !ok {
		k.scopes[scope] = scopeKeys{turnID: turnID, runnerID: k.runnerOf(turnID), started: true}
	}
}
