package ledger

import (
	"maps"
	"slices"
)

// State is a chat's ledger folded: what a decision about the next row needs.
type State struct {
	ChatID   string
	head     int64
	receipts map[string]Prompt // every crowbar prompt, by request id
	pending  []string          // request ids still pending, oldest first
}

// Fold replays rows, in seq order, into chatID's State.
func Fold(chatID string, rows []Row) State {
	s := State{ChatID: chatID}
	for _, r := range rows {
		s.Apply(r)
	}
	return s
}

// Apply advances s by one row. It never fails: a row naming a prompt that is
// unknown or already past that step is late and changes nothing.
func (s *State) Apply(r Row) {
	if r.Seq > s.head {
		s.head = r.Seq
	}
	switch r.Kind {
	case KindUserMessage:
		s.applyUserMessage(r)
	case KindTurnStarted:
		if body, err := DecodeBody[TurnStarted](r); err == nil {
			s.advance(body.RequestID, func(p *Prompt) { p.Status = PromptStatusDelivered })
		}
	case KindPromptSettled:
		if body, err := DecodeBody[PromptSettled](r); err == nil {
			s.advance(body.RequestID, func(p *Prompt) {
				p.Status, p.Consumed, p.Reason = PromptStatusSettled, body.Consumed, body.Reason
			})
		}
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
		RequestID: body.RequestID, TurnID: r.TurnID, Text: body.Text,
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
	return s
}

// Head is the seq of the last row applied; 0 for an empty chat.
func (s State) Head() int64 { return s.head }

// HasOpenWork reports whether something in the chat is still unfinished.
func (s State) HasOpenWork() bool { return len(s.pending) > 0 }

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
