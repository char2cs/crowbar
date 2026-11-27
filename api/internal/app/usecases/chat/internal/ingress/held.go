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

const heldPerChat = 256

const reasonUnclaimedScope = "unclaimed_scope"

type heldFrame struct {
	runner  engineagents.Runner
	agent   engineagents.Agent
	ev      engineagents.CanonicalEvent
	raw     []byte
	channel engineagents.Channel
}

type held struct {
	mu     sync.Mutex
	byChat map[string][]heldFrame
}

func newHeld() *held { return &held{byChat: map[string][]heldFrame{}} }

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

func (h *held) claim(chatID, scope string) []heldFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	var claimed, kept []heldFrame
	for _, f := range h.byChat[chatID] {
		if heldScope(f.ev) == scope {
			claimed = append(claimed, f)
		} else {
			kept = append(kept, f)
		}
	}
	h.setLocked(chatID, kept)
	return claimed
}

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

func (r *Recorder) placeScope(ctx context.Context, chatID string, st ledger.State, f heldFrame, src *Source) (bool, error) {
	switch {
	case handsBackUnbound(st, f.ev):
		r.holdFrame(ctx, chatID, f)
		return true, nil
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

func (r *Recorder) holdFrame(ctx context.Context, chatID string, f heldFrame) {
	if out, full := r.unbound.hold(chatID, f); full {
		r.unclaimed(ctx, chatID, []heldFrame{out})
	}
}

func (r *Recorder) drain(ctx context.Context, chatID, scope string) {
	for _, f := range r.unbound.claim(chatID, scope) {
		if _, _, err := r.Dispatch(ctx, f.runner, f.agent, f.ev, f.raw, f.channel); err != nil {
			slog.WarnContext(ctx, "agent: ingest hook: a held frame of a bound subagent",
				"chat_id", chatID, "scope", scope, "event", f.ev.Kind, "err", err)
			r.deadLetter(ctx, chatID, f, drainReason(err))
		}
	}
}

func (r *Recorder) unclaimed(ctx context.Context, chatID string, frames []heldFrame) {
	for _, f := range frames {
		r.deadLetter(ctx, chatID, f, reasonUnclaimedScope)
	}
}

func (r *Recorder) settleUnclaimed(ctx context.Context, chatID string, frames []heldFrame) {
	var rest []heldFrame
	for _, f := range frames {
		if f.ev.Kind != engineagents.HookSubagentResult {
			rest = append(rest, f)
			continue
		}
		if _, _, err := r.Dispatch(ctx, f.runner, f.agent, f.ev, f.raw, f.channel); err != nil {
			slog.WarnContext(ctx, "agent: ingest hook: a held hand-back nothing bound",
				"chat_id", chatID, "scope", heldScope(f.ev), "err", err)
			r.deadLetter(ctx, chatID, f, drainReason(err))
		}
	}
	r.unclaimed(ctx, chatID, rest)
}

func heldScope(ev engineagents.CanonicalEvent) string {
	if ev.Kind == engineagents.HookSubagentResult && ev.Subagent != nil {
		return ev.Subagent.ID
	}
	return ev.SessionID
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

func drainReason(err error) string {
	switch {
	case errors.Is(err, engineagents.ErrForeignConversation):
		return reasonForeign
	case errors.Is(err, seam.ErrHookNotRecorded):
		return reasonNotRecorded
	}
	return reasonWriteFailed
}
