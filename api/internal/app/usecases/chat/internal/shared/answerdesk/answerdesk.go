// Package answerdesk is the desk of provider prompts currently parked on a human
// decision.
//
// A vendor CLI that needs permission blocks its hook relay and waits for stdout.
// The desk is where that relay is registered so Crowbar's UI can find it, decide
// for it, and hand back exactly the bytes the provider expects — and where a
// relay that nobody answers in time is released so the provider falls back to its
// own terminal UI.
package answerdesk

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Ledger is the conversation record a prompt's outcome is written back to. The
// desk owns that write because it is the only thing that knows WHICH outcome a
// relay reached: answered, proceeded past, or abandoned with its runner.
type Ledger interface {
	ResolveChoice(
		ctx context.Context,
		chatID, choiceID, resolution string,
		now time.Time,
	) error
	// ResolveInterruption closes the notification banner a permission choice
	// opened alongside it (see record's own doc comment) — required so a
	// relay released without an answer doesn't leave that banner counting
	// against the turn's domain.MaxOpenPerTurn for the rest of the turn.
	ResolveInterruption(
		ctx context.Context,
		chatID, id, kind, detail string,
		now time.Time,
	) error
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

// PermissionInterruptionID derives a choice's paired interruption id from the
// choice id alone, by a prefix swap — both are minted from the exact same
// key in turn/observation.go's handleObservation, the one place the pair is
// actually opened, for every choice kind (tool_permission, AskUserQuestion,
// elicitation alike). Every caller that only ever sees the choice id — the
// human answer path (chat.Usecase.AnswerChoice) and this desk's own record,
// both a package away from where the pair was opened — derives the same
// interrupt id from it rather than re-deriving or guessing one; each then
// scopes itself to the choice kinds it actually means to resolve (see their
// own doc comments), since deriving a well-formed id here says nothing about
// whether resolving it is that caller's to do.
func PermissionInterruptionID(choiceID string) string {
	const prefix = "choice-"
	if !strings.HasPrefix(choiceID, prefix) {
		return ""
	}
	return "interrupt-" + strings.TrimPrefix(choiceID, prefix)
}

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

// Desk holds every relay parked on a decision, indexed both ways: by the delivery
// that is blocked (so the relay can find its own verdict) and by the choice the
// user sees (so the UI can answer it).
type Desk struct {
	mu         sync.Mutex
	byChoice   map[string]*Slot
	byDelivery map[string]*Slot

	retention time.Duration
	ledger    Ledger
}

// New returns an empty desk whose undelivered verdicts expire after retention and
// whose outcomes are written back to ledger. A nil ledger is legal and simply
// records nothing, which is what a test that only exercises the desk wants.
func New(retention time.Duration, ledger Ledger) *Desk {
	return &Desk{
		byChoice:   map[string]*Slot{},
		byDelivery: map[string]*Slot{},
		retention:  retention,
		ledger:     ledger,
	}
}

// Hold parks a relay on a prompt and returns the slot now holding it.
//
// A retried delivery gets the slot it already had. A prompt re-asked under a NEW
// delivery displaces the relay already holding it — that relay is released
// unanswered, because the CLI has plainly stopped waiting on it.
func (d *Desk) Hold(deliveryID string, prompt Prompt) *Slot {
	slot := &Slot{Prompt: prompt, done: make(chan struct{})}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
	if existing, held := d.byDelivery[deliveryID]; held {
		return existing
	}
	if stale, held := d.byChoice[prompt.ChoiceID]; held {
		d.forgetLocked(stale)
		stale.settle(nil)
	}
	d.byDelivery[deliveryID] = slot
	d.byChoice[prompt.ChoiceID] = slot
	return slot
}

// Pending reports the prompt a delivery is parked on and how long its relay may
// wait.
func (d *Desk) Pending(deliveryID string) (PendingAnswer, bool) {
	slot, held := d.byDeliveryID(deliveryID)
	if !held {
		return PendingAnswer{}, false
	}
	return PendingAnswer{ChoiceID: slot.ChoiceID, Wait: Wait(slot.Keys.Wait)}, true
}

// Await blocks the relay of deliveryID until a person answers, its declared
// budget expires, or ctx is cancelled. It returns the stdout the relay must
// print — empty when nobody answered in time.
//
// A verdict is claimed by EXACTLY ONE relay: a retried delivery arriving after
// the answer was claimed gets nothing rather than replaying it into a CLI that
// has already acted on it.
func (d *Desk) Await(ctx context.Context, deliveryID string) (HookAnswer, error) {
	slot, held := d.byDeliveryID(deliveryID)
	if !held {
		return HookAnswer{}, nil
	}
	if stdout, claimed := d.claim(slot); claimed {
		return HookAnswer{Stdout: stdout}, nil
	}
	timer := time.NewTimer(Wait(slot.Keys.Wait))
	defer timer.Stop()
	select {
	case <-slot.done:
		stdout, _ := d.claim(slot)
		return HookAnswer{Stdout: stdout}, nil
	case <-timer.C:
		// The budget ran out and a verdict may have landed in the same instant;
		// claim before releasing, or a decision the user already made is thrown away.
		if stdout, claimed := d.claim(slot); claimed {
			return HookAnswer{Stdout: stdout}, nil
		}
		d.release(slot)
		return HookAnswer{}, nil
	case <-ctx.Done():
		d.release(slot)
		return HookAnswer{}, ctx.Err()
	}
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

// ByChoiceID returns the relay holding a choice open, if one still is.
func (d *Desk) ByChoiceID(choiceID string) (*Slot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	slot, ok := d.byChoice[choiceID]
	return slot, ok
}

func (d *Desk) byDeliveryID(deliveryID string) (*Slot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
	slot, ok := d.byDelivery[deliveryID]
	return slot, ok
}

// Resolve records a verdict and wakes the relay. The prompt stops being
// answerable at once, but the slot lingers for the desk's retention so a relay
// that has not asked yet still finds its answer.
func (d *Desk) Resolve(slot *Slot, stdout []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if slot.spent {
		return
	}
	if held, ok := d.byChoice[slot.ChoiceID]; ok && held == slot {
		delete(d.byChoice, slot.ChoiceID)
	}
	slot.decidedAt = time.Now()
	slot.reap = time.AfterFunc(d.retention, d.dropExpired)
	slot.settle(stdout)
}

// Discard retires a relay without printing and reports whether a verdict had
// already been reached. A false verdict is the caller's cue to close the ledger's
// question as proceeded: the provider is about to resolve it through its own UI.
func (d *Desk) Discard(slot *Slot) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	decided := d.claimableLocked(slot)
	d.forgetLocked(slot)
	slot.settle(nil)
	return decided
}

// ReleaseRunner frees every relay of a runner that is gone and closes each
// still-waiting question as abandoned. A relay whose verdict had already landed
// is left alone: it was answered, not abandoned.
//
// It returns the slots it released, so a caller can see what a dead runner took
// with it.
func (d *Desk) ReleaseRunner(ctx context.Context, runnerID string) []*Slot {
	blocked := d.releaseRunner(runnerID)
	for _, slot := range blocked {
		d.record(ctx, slot, domain.ChoiceResolutionAbandoned,
			"agent: answer: release prompt of dead runner")
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
//
// A permission-sourced slot (Event == HookPermission — the same hook both a
// plain tool_permission choice and an AskUserQuestion arrive on) also closes
// its paired interruption here: released without an answer is still decided,
// and leaving that interruption open until turn-close would keep counting it
// against domain.MaxOpenPerTurn for the rest of the turn, same as an
// answered one left unresolved would. Best-effort, same as the choice write
// above — the banner is cosmetic.
func (d *Desk) record(
	ctx context.Context,
	slot *Slot,
	resolution string,
	what string,
) {
	if d.ledger == nil {
		return
	}
	err := d.ledger.ResolveChoice(ctx, slot.ChatID, slot.ChoiceID, resolution, time.Now())
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.WarnContext(ctx, what,
			"chat_id", slot.ChatID, "choice_id", slot.ChoiceID, "err", err)
	}
	if slot.Event != engineagents.HookPermission {
		return
	}
	if id := PermissionInterruptionID(slot.ChoiceID); id != "" {
		if err := d.ledger.ResolveInterruption(
			ctx, slot.ChatID, id, engineagents.InterruptPermission, "", time.Now(),
		); err != nil && !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, what+": interruption",
				"chat_id", slot.ChatID, "choice_id", slot.ChoiceID, "err", err)
		}
	}
}

func (d *Desk) claim(slot *Slot) ([]byte, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.claimableLocked(slot) {
		return nil, false
	}
	d.forgetLocked(slot)
	return slot.stdout, true
}

func (d *Desk) release(slot *Slot) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.claimableLocked(slot) {
		return
	}
	d.forgetLocked(slot)
	slot.settle(nil)
}

func (d *Desk) dropExpired() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
}

func (d *Desk) claimableLocked(slot *Slot) bool {
	return !slot.spent &&
		!slot.decidedAt.IsZero() &&
		time.Since(slot.decidedAt) < d.retention
}

func (d *Desk) forgetLocked(slot *Slot) {
	for id, held := range d.byDelivery {
		if held == slot {
			delete(d.byDelivery, id)
		}
	}
	if held, ok := d.byChoice[slot.ChoiceID]; ok && held == slot {
		delete(d.byChoice, slot.ChoiceID)
	}
	slot.spent = true
	if slot.reap != nil {
		slot.reap.Stop()
		slot.reap = nil
	}
}

func (d *Desk) dropExpiredLocked() {
	var stale []*Slot
	for _, slot := range d.byDelivery {
		if !slot.decidedAt.IsZero() && time.Since(slot.decidedAt) >= d.retention {
			stale = append(stale, slot)
		}
	}
	for _, slot := range stale {
		d.forgetLocked(slot)
	}
}

// slotsOfLocked collects every slot owned by one runner. It walks BOTH indexes:
// a slot whose verdict has landed is gone from byChoice but still on byDelivery
// waiting to be claimed, and a runner's death must free that one too.
func (d *Desk) slotsOfLocked(runnerID string) []*Slot {
	owned := map[*Slot]bool{}
	for _, slot := range d.byChoice {
		owned[slot] = slot.RunnerID == runnerID
	}
	for _, slot := range d.byDelivery {
		owned[slot] = slot.RunnerID == runnerID
	}
	slots := make([]*Slot, 0, len(owned))
	for slot, mine := range owned {
		if mine {
			slots = append(slots, slot)
		}
	}
	return slots
}

// Abandon retires the relay of deliveryID without printing anything, and closes
// its ledger question as proceeded: the provider is about to resolve the prompt
// through its own UI.
//
// Nothing is written when the relay had already been answered — the question was
// decided, not proceeded past — or when no relay was holding the delivery at all.
func (d *Desk) Abandon(ctx context.Context, deliveryID string) {
	slot, held := d.byDeliveryID(deliveryID)
	if !held {
		return
	}
	if decided := d.Discard(slot); decided {
		return
	}
	d.record(ctx, slot, domain.ChoiceResolutionProceeded, "agent: answer: choice resolved elsewhere")
}
