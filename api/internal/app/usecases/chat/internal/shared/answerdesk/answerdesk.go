// Package answerdesk is the desk of provider prompts currently parked on a human
// decision.
//
// A vendor CLI that needs permission blocks its hook relay and waits for stdout.
// The desk is where that relay is registered so Crowbar's UI can find it, decide
// for it, and hand back exactly the bytes the provider expects — and where a
// relay that nobody answers in time is released, its question closed as what its
// channel then hands the provider.
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
	// maxWait caps a declared budget. A provider asking Crowbar to hold a hook open
	// for an hour is holding a process hostage, not asking a question.
	maxWait = 15 * time.Minute
	// DefaultRetention is how long a verdict lingers for a relay that has not asked
	// for it yet. It covers the gap between Crowbar answering and the relay's next
	// poll; past it the answer is stale and the provider has moved on.
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
	// changed is told which chat's answerable prompts just moved, always AFTER the
	// desk's lock is released so it may read the desk back.
	changed func(chatID string)
}

// Option configures a Desk.
type Option func(*Desk)

// WithChange has the desk announce each chat whose set of answerable prompts it
// changed: a relay parked, a verdict landed, a relay released unanswered.
func WithChange(changed func(chatID string)) Option {
	return func(d *Desk) { d.changed = changed }
}

// New returns an empty desk whose undelivered verdicts expire after retention and
// whose outcomes are written back to ledger. A nil ledger is legal and simply
// records nothing, which is what a test that only exercises the desk wants.
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

func (d *Desk) announce(chatID string) {
	if d.changed != nil {
		d.changed(chatID)
	}
}

// Hold parks a relay on prompt, keyed by its ask id, and returns the slot now
// holding it. A prompt asked again displaces the relay already holding it,
// which is released unanswered: the CLI has plainly stopped waiting on it.
func (d *Desk) Hold(prompt Prompt) *Slot {
	slot := &Slot{Prompt: prompt, done: make(chan struct{})}

	defer d.announce(prompt.ChatID)
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

// Pending reports the prompt askID is parked on and how long its relay may
// wait.
func (d *Desk) Pending(askID string) (PendingAnswer, bool) {
	slot, held := d.held(askID)
	if !held {
		return PendingAnswer{}, false
	}
	return PendingAnswer{ChoiceID: slot.ChoiceID, Wait: Wait(slot.Keys.Wait)}, true
}

// AnswerableIDs filters choices down to the ones a relay of chatID is still
// blocked on. It is what makes a pending question actionable in the UI rather
// than merely visible.
func (d *Desk) AnswerableIDs(chatID string, choices []domain.ActivityChoice) []string {
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		if slot, held := d.ByChoiceID(choice.ID); held && slot.ChatID == chatID {
			out = append(out, choice.ID)
		}
	}
	return out
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
	defer d.announce(slot.ChatID)
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
	blocked := d.releaseRunner(runnerID)
	for _, slot := range blocked {
		d.announce(slot.ChatID)
	}
	return blocked
}

func (d *Desk) releaseRunner(runnerID string) []*Slot {
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

// record writes one outcome back to the ledger. A failure is logged and never
// returned: the relay has already been released, and refusing to admit that
// because a write failed would leave the CLI blocked on a person who can no
// longer answer.
func (d *Desk) record(
	ctx context.Context,
	slot *Slot,
	resolution string,
	what string,
) {
	if d.ledger == nil {
		return
	}
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
