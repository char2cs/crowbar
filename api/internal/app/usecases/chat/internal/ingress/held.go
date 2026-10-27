package ingress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// heldPerChat is how many frames of not-yet-bound subagents a chat holds.
const heldPerChat = 256

// reasonUnclaimedScope is a held frame no bind claimed: its parent turn ended,
// or the chat held too many.
const reasonUnclaimedScope = "unclaimed_scope"

// heldFrame is one event of a subagent's own conversation that arrived before
// the call that spawned it was bound to it.
type heldFrame struct {
	runner  engineagents.Runner
	agent   engineagents.Agent
	ev      engineagents.CanonicalEvent
	raw     []byte
	channel engineagents.Channel
}

// held is each chat's frames of subagents not yet bound, in arrival order.
// Every frame leaves it drained on its bind or dead-lettered.
type held struct {
	mu     sync.Mutex
	byChat map[string][]heldFrame
}

func newHeld() *held { return &held{byChat: map[string][]heldFrame{}} }

// hold keeps f for chatID and returns the frame it pushed out, if the chat
// already held as many as it may.
func (h *held) hold(chatID string, f heldFrame) (heldFrame, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	frames := append(h.byChat[chatID], f)
	if len(frames) <= heldPerChat {
		h.byChat[chatID] = frames
		return heldFrame{}, false
	}
	h.byChat[chatID] = frames[1:]
	return frames[0], true
}

// claim takes chatID's frames of scope, in arrival order.
func (h *held) claim(chatID, scope string) []heldFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	var claimed, kept []heldFrame
	for _, f := range h.byChat[chatID] {
		if f.ev.SessionID == scope {
			claimed = append(claimed, f)
		} else {
			kept = append(kept, f)
		}
	}
	h.setLocked(chatID, kept)
	return claimed
}

// releaseRunner takes chatID's frames runnerID sent.
func (h *held) releaseRunner(chatID, runnerID string) []heldFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out, kept []heldFrame
	for _, f := range h.byChat[chatID] {
		if f.runner.ID == runnerID {
			out = append(out, f)
		} else {
			kept = append(kept, f)
		}
	}
	h.setLocked(chatID, kept)
	return out
}

// release takes every frame chatID holds.
func (h *held) release(chatID string) []heldFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	frames := h.byChat[chatID]
	delete(h.byChat, chatID)
	return frames
}

func (h *held) setLocked(chatID string, frames []heldFrame) {
	if len(frames) == 0 {
		delete(h.byChat, chatID)
		return
	}
	h.byChat[chatID] = frames
}

// placeScope places f when it names another conversation than the chat's: in
// the subagent bound to it, else held while one is starting (held), else foreign.
func (r *Recorder) placeScope(ctx context.Context, chatID string, st ledger.State, f heldFrame, src *Source) (bool, error) {
	switch {
	case !namesAnotherConversation(src.SessionID, f.ev, f.channel):
		return false, nil
	case st.ScopeOpen(f.ev.SessionID):
		src.Scope = f.ev.SessionID
		return false, nil
	case awaitsBind(st, f.ev):
		r.holdFrame(ctx, chatID, f)
		return true, nil
	}
	return false, fmt.Errorf("agent: hook: session %s: %w", f.ev.SessionID, engineagents.ErrForeignConversation)
}

// holdFrame keeps an event of a subagent not yet bound until its bind, and
// dead-letters the oldest frame a full chat pushes out.
func (r *Recorder) holdFrame(ctx context.Context, chatID string, f heldFrame) {
	if out, full := r.unbound.hold(chatID, f); full {
		r.unclaimed(ctx, chatID, []heldFrame{out})
	}
}

// drain records the frames held for scope now that it is bound. The caller
// holds the chat's lock; each frame is dispatched as if it had just arrived.
func (r *Recorder) drain(ctx context.Context, chatID, scope string) {
	for _, f := range r.unbound.claim(chatID, scope) {
		if _, _, err := r.Dispatch(ctx, f.runner, f.agent, f.ev, f.raw, f.channel); err != nil {
			slog.WarnContext(ctx, "agent: ingest hook: a held frame of a bound subagent",
				"chat_id", chatID, "scope", scope, "event", f.ev.Kind, "err", err)
			r.deadLetter(ctx, chatID, f, drainReason(err))
		}
	}
}

// unclaimed dead-letters frames no bind claimed.
func (r *Recorder) unclaimed(ctx context.Context, chatID string, frames []heldFrame) {
	for _, f := range frames {
		r.deadLetter(ctx, chatID, f, reasonUnclaimedScope)
	}
}

func (r *Recorder) deadLetter(ctx context.Context, chatID string, f heldFrame, reason string) {
	err := r.ledger.DeadLetter(context.WithoutCancel(ctx), ledgerstore.DeadLetter{
		ChatID: chatID, RunnerID: f.runner.ID, Provider: f.runner.ProviderID, Event: f.ev.Kind,
		Reason: reason, Payload: f.raw,
	})
	if err != nil {
		slog.ErrorContext(ctx, "agent: ingest hook: dead-letter a held frame", "chat_id", chatID, "err", err)
	}
}

// drainReason is why a drained frame could not be recorded, as the ingress
// would have dead-lettered it on arrival.
func drainReason(err error) string {
	switch {
	case errors.Is(err, errInternalSubagent):
		return reasonInternal
	case errors.Is(err, engineagents.ErrForeignConversation):
		return reasonForeign
	case errors.Is(err, seam.ErrHookNotRecorded):
		return reasonNotRecorded
	}
	return reasonWriteFailed
}
