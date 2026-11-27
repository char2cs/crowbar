package answerdesk

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type Ledger interface {
	ResolveAsk(ctx context.Context, chatID, askID, resolution string) error
}

const (
	waitFallback     = 120 * time.Second
	maxWait          = 24 * time.Hour
	DefaultRetention = time.Minute
)

func Wait(declared time.Duration) time.Duration {
	switch {
	case declared <= 0:
		return waitFallback
	case declared > maxWait:
		return maxWait
	default:
		return declared
	}
}

type Desk struct {
	mu    sync.Mutex
	slots map[string]*Slot

	retention time.Duration
	ledger    Ledger
	after     func(time.Duration) <-chan time.Time
}

type Option func(*Desk)

func WithBudgetClock(after func(time.Duration) <-chan time.Time) Option {
	return func(d *Desk) { d.after = after }
}

func New(retention time.Duration, ledger Ledger, opts ...Option) *Desk {
	d := &Desk{
		slots:     map[string]*Slot{},
		retention: retention,
		ledger:    ledger,
		after:     time.After,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func (d *Desk) Hold(prompt Prompt) *Slot {
	slot := &Slot{Prompt: prompt, done: make(chan struct{})}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
	if stale, held := d.slots[prompt.ChoiceID]; held {
		d.forgetLocked(stale)
		stale.settle(nil)
		if c := stale.answering; c != nil {
			stale.answering, slot.answering, c.slot = nil, c, slot
		}
	}
	d.slots[prompt.ChoiceID] = slot
	return slot
}

func (d *Desk) Holds(askID string) bool {
	_, held := d.held(askID)
	return held
}

func (d *Desk) ByChoiceID(choiceID string) (*Slot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	slot, ok := d.slots[choiceID]
	if !ok || !slot.decidedAt.IsZero() {
		return nil, false
	}
	return slot, true
}

func (d *Desk) held(askID string) (*Slot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
	slot, ok := d.slots[askID]
	return slot, ok
}

func (d *Desk) Discard(slot *Slot) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	decided := d.claimableLocked(slot)
	d.forgetLocked(slot)
	slot.settle(nil)
	return decided
}

func (d *Desk) ReleaseRunner(runnerID string) []*Slot {
	d.mu.Lock()
	defer d.mu.Unlock()

	var blocked []*Slot
	for _, slot := range d.slotsOfLocked(runnerID) {
		decided := !slot.decidedAt.IsZero()
		d.forgetLocked(slot)
		if decided {
			continue
		}
		slot.settle(nil)
		blocked = append(blocked, slot)
	}
	return blocked
}

func (d *Desk) record(
	ctx context.Context,
	slot *Slot,
	resolution string,
	what string,
) {
	err := d.ledger.ResolveAsk(ctx, slot.ChatID, slot.ChoiceID, resolution)
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.WarnContext(ctx, what, "chat_id", slot.ChatID, "choice_id", slot.ChoiceID, "err", err)
	}
}

func (d *Desk) dropExpired() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
}

func (d *Desk) forgetLocked(slot *Slot) {
	if held, ok := d.slots[slot.ChoiceID]; ok && held == slot {
		delete(d.slots, slot.ChoiceID)
	}
	slot.spent = true
	if slot.reap != nil {
		slot.reap.Stop()
		slot.reap = nil
	}
}

func (d *Desk) dropExpiredLocked() {
	for _, slot := range d.slots {
		if !slot.decidedAt.IsZero() && time.Since(slot.decidedAt) >= d.retention {
			d.forgetLocked(slot)
		}
	}
}

func (d *Desk) slotsOfLocked(runnerID string) []*Slot {
	var slots []*Slot
	for _, slot := range d.slots {
		if slot.RunnerID == runnerID {
			slots = append(slots, slot)
		}
	}
	return slots
}

func (d *Desk) Abandon(ctx context.Context, askID string) {
	slot, held := d.held(askID)
	if !held {
		return
	}
	if decided := d.Discard(slot); decided {
		return
	}
	d.record(ctx, slot, domain.ChoiceResolutionProceeded, "agent: answer: choice resolved elsewhere")
}
