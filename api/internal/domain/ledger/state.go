package ledger

import (
	"maps"
	"slices"
	"strings"
)

// State is a chat's ledger folded: what a decision about the next row needs.
type State struct {
	ChatID   string
	head     int64
	receipts map[string]Prompt // every crowbar prompt, by request id
	pending  []string          // request ids still pending, oldest first

	conversations []Conversation // every (provider, session) bound, by first binding
	current       map[string]int // provider → its conversation's index in conversations

	live     RunnerStarted // the runner started and not exited, while hasLive
	hasLive  bool
	lastExit RunnerExit // the newest runner_exited, while hasExit
	hasExit  bool

	turn    turnState // the newest main-scope turn, while hasTurn
	hasTurn bool
	bg      background // the live runner's last restated background work

	keys keys // what a retried hook's fact is recognised by
}

// keepTurns is how many of the newest main turns keep their keys: a retried
// or late hook names the running turn or the one before it.
const keepTurns = 2

// keys are the natural keys of the newest turns' facts.
type keys struct {
	turns      []keptTurn        // the kept turns, oldest first
	provider   map[string]string // provider turn id → turn id
	messages   map[string]messageKeys
	lastMain   map[string]string // turn → its newest main-scope message
	replies    map[string]reply
	tools      map[string]toolKeys
	scopes     map[string]scopeKeys
	asks       map[string]PendingAsk
	compaction compaction
}

type keptTurn struct{ id, runnerID string }

func (k *keys) init() *keys {
	if k.provider == nil {
		k.provider, k.messages, k.lastMain = map[string]string{}, map[string]messageKeys{}, map[string]string{}
		k.replies, k.tools = map[string]reply{}, map[string]toolKeys{}
		k.scopes, k.asks = map[string]scopeKeys{}, map[string]PendingAsk{}
	}
	return k
}

func (k keys) clone() keys {
	k.turns = slices.Clone(k.turns)
	k.provider, k.messages, k.lastMain = maps.Clone(k.provider), maps.Clone(k.messages), maps.Clone(k.lastMain)
	k.replies, k.tools = maps.Clone(k.replies), maps.Clone(k.tools)
	k.scopes, k.asks = maps.Clone(k.scopes), maps.Clone(k.asks)
	return k
}

// openTurn keeps turnID's keys and drops those of the turn it pushes out.
func (k *keys) openTurn(turnID, providerTurnID, runnerID string) {
	k.init()
	if providerTurnID != "" {
		k.provider[providerTurnID] = turnID
	}
	k.turns = append(slices.Clip(k.turns), keptTurn{id: turnID, runnerID: runnerID})
	if len(k.turns) <= keepTurns {
		return
	}
	gone := k.turns[0].id
	k.turns = k.turns[1:]
	maps.DeleteFunc(k.provider, func(_, t string) bool { return t == gone })
	maps.DeleteFunc(k.messages, func(_ string, m messageKeys) bool { return m.turnID == gone })
	maps.DeleteFunc(k.tools, func(_ string, t toolKeys) bool { return t.turnID == gone })
	maps.DeleteFunc(k.scopes, func(_ string, s scopeKeys) bool { return s.turnID == gone && s.stopped })
	maps.DeleteFunc(k.asks, func(_ string, a PendingAsk) bool { return a.TurnID == gone })
	delete(k.lastMain, gone)
	delete(k.replies, gone)
}

// ProviderTurn is the kept turn the provider names providerTurnID.
func (s State) ProviderTurn(providerTurnID string) (string, bool) {
	id, ok := s.keys.provider[providerTurnID]
	return id, ok && providerTurnID != ""
}

// RunnerTurn is the newest kept turn runnerID ran.
func (s State) RunnerTurn(runnerID string) (string, bool) {
	for i := len(s.keys.turns) - 1; i >= 0; i-- {
		if t := s.keys.turns[i]; t.runnerID == runnerID && runnerID != "" {
			return t.id, true
		}
	}
	return "", false
}

// ref keys a fact by its scope and its own id.
func ref(scope, id string) string { return scope + "\x00" + id }

func unref(key string) (scope, id string) {
	scope, id, _ = strings.Cut(key, "\x00")
	return scope, id
}

// Fold replays rows, in seq order, into chatID's State.
func Fold(chatID string, rows []Row) State {
	s := State{ChatID: chatID}
	for _, r := range rows {
		s.Apply(r)
	}
	return s
}

// appliers advance a State by a row of their kind; a kind with none keeps no key.
var appliers = map[Kind]func(*State, Row){
	KindUserMessage:   (*State).applyUserMessage,
	KindTurnStarted:   (*State).startTurn,
	KindTurnEnded:     func(s *State, r Row) { s.applyTurnEnded(r); s.outliveScopes(r) },
	KindTurnFailed:    func(s *State, r Row) { s.applyTurnFailed(r); s.outliveScopes(r) },
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
}

// Apply advances s by one row. It never fails: a row naming a prompt that is
// unknown or already past that step is late and changes nothing.
func (s *State) Apply(r Row) {
	if r.Seq > s.head {
		s.head = r.Seq
	}
	if apply, ok := appliers[r.Kind]; ok {
		apply(s, r)
	}
}

func (s *State) startTurn(r Row) {
	if body, err := DecodeBody[TurnStarted](r); err == nil && s.startsTurn(r) {
		s.advance(body.RequestID, func(p *Prompt) { p.Status = PromptStatusDelivered })
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

// advance moves a pending prompt on; any other prompt is past that step.
func (s *State) advance(requestID string, move func(*Prompt)) {
	p, ok := s.receipts[requestID]
	if !ok || p.Status != PromptStatusPending {
		return
	}
	move(&p)
	s.receipts[requestID] = p
	s.pending = slices.DeleteFunc(s.pending, func(id string) bool { return id == requestID })
}

// Clone is a State that shares nothing with s, safe to hand to another goroutine.
func (s State) Clone() State {
	s.receipts = maps.Clone(s.receipts)
	s.pending = slices.Clone(s.pending)
	s.conversations = slices.Clone(s.conversations)
	s.current = maps.Clone(s.current)
	s.keys = s.keys.clone()
	return s
}

// Head is the seq of the last row applied; 0 for an empty chat.
func (s State) Head() int64 { return s.head }

// HasOpenWork reports whether something in the chat is still unfinished: a
// prompt awaiting the provider, a running turn, or a runner that has not exited.
func (s State) HasOpenWork() bool {
	_, running := s.RunningTurn()
	return len(s.pending) > 0 || running || s.hasLive
}

// PendingPrompts are the crowbar prompts not yet acknowledged, oldest first.
func (s State) PendingPrompts() []Prompt {
	out := make([]Prompt, 0, len(s.pending))
	for _, id := range s.pending {
		out = append(out, s.receipts[id])
	}
	return out
}

// Receipt is the crowbar prompt recorded under requestID, in any status.
func (s State) Receipt(requestID string) (Prompt, bool) {
	p, ok := s.receipts[requestID]
	return p, ok
}
