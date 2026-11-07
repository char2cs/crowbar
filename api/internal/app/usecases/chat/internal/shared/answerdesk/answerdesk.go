// Package answerdesk holds provider prompts parked on a human decision. A CLI asking permission
// holds its hook's HTTP request open; the desk lets the UI answer with exactly the response bytes
// the provider expects, and releases a hook nobody answers in time.
package answerdesk

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Ledger closes an ask its relay proceeded past. The desk owns that write
// because it alone knows the relay reached it.
type Ledger interface {
	ResolveAsk(ctx context.Context, chatID, askID, resolution string) error
}

const (
	// waitFallback is how long a relay waits when its provider declared no budget.
	waitFallback = 120 * time.Second
	// maxWait caps a declared budget. A person may take hours over a plan, and a
	// lapsed hold strands the CLI on a dialog only its terminal answers.
	maxWait = 24 * time.Hour
	// DefaultRetention is how long a verdict lingers for a relay that has not asked
	// for it yet: one answered between its hook being recorded and its wait
	// starting. Past it the answer is stale and the provider has moved on.
	DefaultRetention = time.Minute
)

// Wait clamps a provider's declared answer budget: absent or negative falls back,
// absurd is capped, anything sane is honoured.
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

// Desk holds every relay parked on a decision, by the id of the ask it is
// parked on: the relay finds its verdict by it, and the UI answers it by it.
type Desk struct {
	mu    sync.Mutex
	slots map[string]*Slot

	retention time.Duration
	ledger    Ledger
	// after is the clock a relay's budget runs out on.
	after func(time.Duration) <-chan time.Time
}

// Option configures a Desk.
type Option func(*Desk)

// WithBudgetClock runs every relay's budget out when after's channel fires.
func WithBudgetClock(after func(time.Duration) <-chan time.Time) Option {
	return func(d *Desk) { d.after = after }
}

// New returns an empty desk whose undelivered verdicts expire after retention and
// whose outcomes are written back to ledger.
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

// Hold parks a relay on prompt, keyed by its ask id, and returns the slot now
// holding it. A prompt asked again displaces the relay already holding it,
// which is released unanswered: the CLI has plainly stopped waiting on it.
func (d *Desk) Hold(prompt Prompt) *Slot {
	slot := &Slot{Prompt: prompt, done: make(chan struct{})}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
	if stale, held := d.slots[prompt.ChoiceID]; held {
		d.forgetLocked(stale)
		stale.settle(nil)
		if c := stale.answering; c != nil {
			// An answer being recorded is the ask's, so it goes to its new relay.
			stale.answering, slot.answering, c.slot = nil, c, slot
		}
	}
	d.slots[prompt.ChoiceID] = slot
	return slot
}

// Holds reports whether askID is on the desk: still open, or decided with a
// verdict its hook has not collected yet; either way the hook must be held.
func (d *Desk) Holds(askID string) bool {
	_, held := d.held(askID)
	return held
}

// ByChoiceID returns the relay holding a choice open, while it is undecided.
func (d *Desk) ByChoiceID(choiceID string) (*Slot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	slot, ok := d.slots[choiceID]
	if !ok || !slot.decidedAt.IsZero() {
		return nil, false
	}
	return slot, true
}

// held is the slot askID is parked on, decided or not.
func (d *Desk) held(askID string) (*Slot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
	slot, ok := d.slots[askID]
	return slot, ok
}

// Discard retires a relay without printing and reports whether a verdict had
// already been reached. A false verdict is the caller's cue to close the question
// as proceeded: the provider is resolving it.
func (d *Desk) Discard(slot *Slot) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	decided := d.claimableLocked(slot)
	d.forgetLocked(slot)
	slot.settle(nil)
	return decided
}

// ReleaseRunner frees every still-waiting relay of a runner that is gone, and
// returns them. It writes nothing: the runner's exit closes its questions. A
// relay whose verdict had already landed is left alone.
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

// record writes one outcome to the ledger. A failure is logged, not returned: the relay is already
// released, and failing here would leave the CLI blocked on a person who can no longer answer.
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

// slotsOfLocked collects every slot owned by one runner, decided or not: a
// runner's death frees a verdict still waiting to be claimed too.
func (d *Desk) slotsOfLocked(runnerID string) []*Slot {
	var slots []*Slot
	for _, slot := range d.slots {
		if slot.RunnerID == runnerID {
			slots = append(slots, slot)
		}
	}
	return slots
}

// Abandon retires the relay held on askID without printing anything, and
// closes its question as proceeded unless it was answered or never held. An
// answer still being written then loses to this close, or wins it, in the ledger.
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
