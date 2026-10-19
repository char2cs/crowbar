package turn

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// hookReplyTTL covers the gap between a delivery's ingest and its relay reading
// the answer; past it the relay has given up and the reply is stale.
const hookReplyTTL = time.Minute

// hookReplies holds the stdout each relayed delivery must print, by delivery id.
// It is read, never consumed, so a relay retried after a lost response still
// gets the same answer.
type hookReplies struct {
	mu      sync.Mutex
	replies map[string]hookReply
}

type hookReply struct {
	stdout string
	at     time.Time
}

func (r *hookReplies) put(deliveryID, stdout string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.replies == nil {
		r.replies = map[string]hookReply{}
	}
	now := time.Now()
	for id, held := range r.replies {
		if now.Sub(held.at) > hookReplyTTL {
			delete(r.replies, id)
		}
	}
	r.replies[deliveryID] = hookReply{stdout: stdout, at: now}
}

func (r *hookReplies) get(deliveryID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	held, ok := r.replies[deliveryID]
	return held.stdout, ok
}

// TakeHookReply is the stdout the relay of deliveryID must print, if ingesting
// it produced one.
func (t *Turns) TakeHookReply(deliveryID string) (string, bool) {
	return t.replies.get(deliveryID)
}

// steerTexts holds each chat's steered prompt's dispatch text until the end
// hook of the turn it rides delivers it: the ledger keeps the text the user
// wrote, and the CLI is handed the rewritten one.
type steerTexts struct {
	mu     sync.Mutex
	byChat map[string]steerText
}

type steerText struct{ requestID, text string }

func (s *steerTexts) put(chatID, requestID, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byChat == nil {
		s.byChat = map[string]steerText{}
	}
	s.byChat[chatID] = steerText{requestID: requestID, text: text}
}

// take removes and returns the chat's held dispatch text.
func (s *steerTexts) take(chatID string) steerText {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := s.byChat[chatID]
	delete(s.byChat, chatID)
	return held
}

// HoldSteer keeps dispatchText for requestID, a prompt about to be recorded
// as steered on chatID's running turn.
func (t *Turns) HoldSteer(chatID, requestID, dispatchText string) {
	t.steers.put(chatID, requestID, dispatchText)
}

// pendingSteer is the steered prompt runnerID carries on its running turn.
func pendingSteer(st ledger.State, runnerID string) (ledger.Prompt, bool) {
	for _, p := range st.PendingPrompts() {
		if p.Delivery == ledger.DeliverySteer && p.RunnerID == runnerID {
			return p, true
		}
	}
	return ledger.Prompt{}, false
}

// settleSteer settles the steered prompt runnerID carries for reason.
func settleSteer(st ledger.State, runnerID, reason string) ([]ledger.Row, error) {
	return ledger.Settle(st, func(p ledger.Prompt) bool {
		return p.Delivery == ledger.DeliverySteer && p.RunnerID == runnerID
	}, false, reason)
}

// steerReply is the hook reply that hands p to the CLI as its turn ends, or
// false when the relay cannot carry one.
func (t *Turns) steerReply(ctx context.Context, chatID string, p ledger.Prompt, agent engineagents.Agent) (string, bool) {
	text := p.Text
	if held := t.steers.take(chatID); held.requestID == p.RequestID && held.text != "" {
		text = held.text
	}
	stdout, ok := agent.PromptSteer(text)
	return stdout, ok && inflight.DeliveryID(ctx) != ""
}

// recordSteered records a delivered steered prompt in the conversation record
// as the user turn the CLI's continuation answers.
func (t *Turns) recordSteered(ctx context.Context, chat domain.Chat, runner engineagents.Runner, p ledger.Prompt) {
	err := t.conversations.AppendRunnerTurn(
		inflight.WithRecordID(ctx, p.RequestID), chat, runner.ProviderID, runner.ID,
		t.boundSession(ctx, chat.ID, runner.ProviderID), domain.TurnRoleUser, p.Text,
	)
	t.openAssistantTurn(ctx, chat, runner)
	note(ctx, "record steered prompt", err)
}
