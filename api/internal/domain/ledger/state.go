package ledger

import (
	"maps"
	"slices"
	"strings"
	"time"
)

type State struct {
	ChatID   string
	head     int64
	receipts map[string]Prompt
	pending  []string

	conversations []Conversation
	current       map[string]int
	clearedAt     time.Time

	live     RunnerStarted
	hasLive  bool
	launches map[string]RunnerStarted
	lastExit RunnerExit
	hasExit  bool

	turn    turnState
	hasTurn bool
	agents  SubagentBook
	plan    []PlanItem
	effort  string
	moves   moves
	shells  map[string]Shell

	keys keys
}

const keepTurns = 2

type keys struct {
	turns      []keptTurn
	provider   map[string]string
	messages   map[string]messageKeys
	lastMain   map[string]string
	tools      map[string]toolKeys
	asks       map[string]PendingAsk
	compaction compaction
}

type keptTurn struct{ id, runnerID string }

func (k *keys) init() *keys {
	if k.provider == nil {
		k.provider, k.messages, k.lastMain = map[string]string{}, map[string]messageKeys{}, map[string]string{}
		k.tools, k.asks = map[string]toolKeys{}, map[string]PendingAsk{}
	}
	return k
}

func (k keys) clone() keys {
	k.turns = slices.Clone(k.turns)
	k.provider, k.messages, k.lastMain = maps.Clone(k.provider), maps.Clone(k.messages), maps.Clone(k.lastMain)
	k.tools, k.asks = maps.Clone(k.tools), maps.Clone(k.asks)
	return k
}

func (k *keys) openTurn(turnID, providerTurnID, runnerID string) string {
	k.init()
	if providerTurnID != "" {
		k.provider[providerTurnID] = turnID
	}
	k.turns = append(slices.Clip(k.turns), keptTurn{id: turnID, runnerID: runnerID})
	if len(k.turns) <= keepTurns {
		return ""
	}
	gone := k.turns[0].id
	k.turns = k.turns[1:]
	maps.DeleteFunc(k.provider, func(_, t string) bool { return t == gone })
	maps.DeleteFunc(k.messages, func(_ string, m messageKeys) bool { return m.turnID == gone })
	maps.DeleteFunc(k.tools, func(_ string, t toolKeys) bool { return t.turnID == gone })
	maps.DeleteFunc(k.asks, func(_ string, a PendingAsk) bool { return a.TurnID == gone })
	delete(k.lastMain, gone)
	return gone
}

func (k keys) runnerOf(turnID string) string {
	for _, t := range k.turns {
		if t.id == turnID {
			return t.runnerID
		}
	}
	return ""
}

func (s State) ProviderTurn(providerTurnID string) (string, bool) {
	id, ok := s.keys.provider[providerTurnID]
	return id, ok && providerTurnID != ""
}

func (s State) RunnerTurn(runnerID string) (string, bool) {
	for i := len(s.keys.turns) - 1; i >= 0; i-- {
		if t := s.keys.turns[i]; t.runnerID == runnerID && runnerID != "" {
			return t.id, true
		}
	}
	return "", false
}

func ref(scope, id string) string { return scope + "\x00" + id }

func unref(key string) (scope, id string) {
	scope, id, _ = strings.Cut(key, "\x00")
	return scope, id
}

func Fold(chatID string, rows []Row) State {
	s := State{ChatID: chatID}
	for _, r := range rows {
		s.Apply(r)
	}
	return s
}

var appliers = map[Kind]func(*State, Row){
	KindUserMessage:   (*State).applyUserMessage,
	KindTurnStarted:   (*State).startTurn,
	KindTurnEnded:     (*State).applyTurnEnded,
	KindTurnFailed:    (*State).applyTurnFailed,
	KindPromptSettled: (*State).applyPromptSettled,
	KindSessionBound:  (*State).applySessionBound,
	KindSessionLost:   (*State).applySessionLost,
	KindMarker:        (*State).applyMarker,
	KindRunnerStarted: (*State).applyRunnerStarted,
	KindRunnerExited:  (*State).applyRunnerExited,
	KindAssistantText: (*State).applyAssistantText,
	KindToolCall:      (*State).applyToolCall,
	KindAsk:           (*State).applyAsk,
	KindAskResolved:   (*State).applyAskResolved,
	KindSubagent:      (*State).applySubagent,
	KindPlan:          (*State).applyPlan,
	KindMove:          (*State).applyMove,
	KindShell:         (*State).applyShell,
}

func (s *State) Apply(r Row) {
	if r.Seq > s.head {
		s.head = r.Seq
	}
	if apply, ok := appliers[r.Kind]; ok {
		apply(s, r)
	}
	s.agents.Apply(r)
}

func (s *State) startTurn(r Row) {
	if body, err := DecodeBody[TurnStarted](r); err == nil && s.startsTurn(r) {
		s.advance(body.RequestID, func(p *Prompt) { p.Status = PromptStatusDelivered })
		s.noteDelivered(body.RequestID)
		s.applyTurnStarted(r, body)
	}
}

func (s *State) applyPromptSettled(r Row) {
	if body, err := DecodeBody[PromptSettled](r); err == nil {
		s.advance(body.RequestID, func(p *Prompt) {
			p.Status, p.Consumed, p.Reason = PromptStatusSettled, body.Consumed, body.Reason
		})
	}
}

func (s *State) applyUserMessage(r Row) {
	body, err := DecodeBody[UserMessage](r)
	if err != nil || body.Origin != OriginCrowbar || body.RequestID == "" {
		return
	}
	if _, seen := s.receipts[body.RequestID]; seen {
		return
	}
	if s.receipts == nil {
		s.receipts = map[string]Prompt{}
	}
	s.receipts[body.RequestID] = Prompt{
		RequestID: body.RequestID, TurnID: r.TurnID, Text: body.Text, Dispatch: body.Dispatch,
		Delivery: body.Delivery, RunnerID: body.RunnerID, Status: PromptStatusPending, At: r.At,
	}
	s.pending = append(s.pending, body.RequestID)
}

func (s *State) advance(requestID string, move func(*Prompt)) {
	p, ok := s.receipts[requestID]
	if !ok || p.Status != PromptStatusPending {
		return
	}
	move(&p)
	s.receipts[requestID] = p
	s.pending = slices.DeleteFunc(s.pending, func(id string) bool { return id == requestID })
}

func (s State) Clone() State {
	s.receipts = maps.Clone(s.receipts)
	s.pending = slices.Clone(s.pending)
	s.conversations = slices.Clone(s.conversations)
	s.current = maps.Clone(s.current)
	s.launches = maps.Clone(s.launches)
	s.keys = s.keys.clone()
	s.agents = s.agents.clone()
	s.plan = slices.Clone(s.plan)
	s.moves = s.moves.clone()
	s.shells = maps.Clone(s.shells)
	return s
}

func (s State) Head() int64 { return s.head }

func (s State) HasOpenWork() bool {
	_, running := s.RunningTurn()
	return len(s.pending) > 0 || running || s.hasLive || len(s.Subagents()) > 0 || s.moves.pending != nil
}

func (s State) PendingPrompts() []Prompt {
	out := make([]Prompt, 0, len(s.pending))
	for _, id := range s.pending {
		out = append(out, s.receipts[id])
	}
	return out
}

func (s State) Receipt(requestID string) (Prompt, bool) {
	p, ok := s.receipts[requestID]
	return p, ok
}
