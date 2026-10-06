package turn

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
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

// QueueSteer parks s on runnerID's running turn so the turn's end hook delivers
// it. It takes the runner's hook gate: either the end hook has not been ingested
// yet and will find s, or it already completed and the turn is no longer open.
func (t *Turns) QueueSteer(runnerID string, s inflight.Steered) bool {
	defer t.hookGates.Lock(runnerID)()
	return t.turns.Steer(runnerID, s)
}

// deliverSteered answers the turn-end hook with the parked message and records
// it as the user turn the CLI's continuation is about to answer. The CLI never
// fires a prompt hook for it, so this is the only place the turn opens.
func (t *Turns) deliverSteered(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	s inflight.Steered,
) {
	stdout, ok := agent.PromptSteer(s.DispatchText)
	deliveryID := inflight.DeliveryID(ctx)
	if !ok || deliveryID == "" {
		t.runners.RefuseSteered(ctx, s)
		return
	}
	t.replies.put(deliveryID, stdout)
	if err := t.recordUserTurn(ctx, chat, runner, agent, s.Text); err != nil {
		slog.WarnContext(ctx, "agent: record steered prompt (delivered anyway)",
			"chat_id", chat.ID, "runner_id", runner.ID, "client_request_id", s.RequestID, "err", err)
	}
}
