package answerdesk

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

// ErrNotWaiting refuses an answer whose relay stopped waiting first: its budget
// ran out, it was released, or another answer claimed it.
var ErrNotWaiting = fmt.Errorf("%w: this prompt can no longer be answered from Crowbar", apperr.ErrConflict)

// Await blocks the relay held on askID until a person answers, claimed by exactly
// one relay; until its budget runs out, closing the question as onExpiry, what the
// relay's channel then hands its CLI; or until ctx ends, leaving the ask to Abandon.
func (d *Desk) Await(ctx context.Context, askID, onExpiry string) (HookAnswer, error) {
	slot, held := d.held(askID)
	if !held {
		return HookAnswer{}, nil
	}
	if stdout, claimed := d.take(slot); claimed {
		return HookAnswer{Stdout: stdout}, nil
	}
	select {
	case <-slot.done:
		stdout, _ := d.take(slot)
		return HookAnswer{Stdout: stdout}, nil
	case <-d.after(Wait(slot.Keys.Wait)):
		return d.expire(ctx, slot, onExpiry)
	case <-ctx.Done():
		return HookAnswer{}, ctx.Err()
	}
}

// expire ends slot's wait once its budget ran out. An answer being recorded owns
// the relay, so expire waits for it, and releases the relay only if it is withdrawn.
func (d *Desk) expire(ctx context.Context, slot *Slot, onExpiry string) (HookAnswer, error) {
	for {
		stdout, answering, released := d.lapse(slot)
		if released {
			d.record(context.WithoutCancel(ctx), slot, onExpiry, "agent: answer: budget ran out")
		}
		if answering == nil {
			return HookAnswer{Stdout: stdout}, nil
		}
		select {
		case <-answering:
		case <-ctx.Done():
			return HookAnswer{}, ctx.Err()
		}
	}
}

// lapse is slot's budget running out: its verdict if one landed, the answer
// claiming it if one is being recorded, else its release, unless another party
// has already released it.
func (d *Desk) lapse(slot *Slot) (stdout []byte, answering <-chan struct{}, released bool) {
	defer func() {
		if released {
			d.announce(slot.ChatID)
		}
	}()
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case d.claimableLocked(slot):
		d.forgetLocked(slot)
		return slot.stdout, nil, false
	case slot.answering != nil:
		return nil, slot.answering, false
	case slot.spent:
		return nil, nil, false
	}
	d.forgetLocked(slot)
	slot.settle(nil)
	return nil, nil, true
}

// Answer hands stdout to slot's relay if record, run while the relay is claimed
// for this answer alone, reports it recorded. A relay that stopped waiting first
// refuses it with ErrNotWaiting before record runs; a failed record hands it back.
func (d *Desk) Answer(
	ctx context.Context, slot *Slot, stdout []byte, record func(context.Context) (bool, error),
) (bool, error) {
	if !d.claim(slot) {
		return false, ErrNotWaiting
	}
	recorded, err := record(ctx)
	if err != nil || !recorded {
		d.unclaim(slot)
		return recorded, err
	}
	d.resolve(slot, stdout)
	return true, nil
}

func (d *Desk) claim(slot *Slot) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if slot.spent || !slot.decidedAt.IsZero() || slot.answering != nil {
		return false
	}
	slot.answering = make(chan struct{})
	return true
}

// unclaim hands slot back to its relay's budget.
func (d *Desk) unclaim(slot *Slot) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.endClaimLocked(slot)
}

func (d *Desk) endClaimLocked(slot *Slot) {
	if slot.answering != nil {
		close(slot.answering)
		slot.answering = nil
	}
}

// resolve records a verdict and wakes the relay. The prompt stops being
// answerable at once, but the slot lingers for the desk's retention so a relay
// that has not asked yet still finds its answer.
func (d *Desk) resolve(slot *Slot, stdout []byte) {
	defer d.announce(slot.ChatID)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.endClaimLocked(slot)
	if slot.spent || !slot.decidedAt.IsZero() {
		return
	}
	slot.decidedAt = time.Now()
	slot.reap = time.AfterFunc(d.retention, d.dropExpired)
	slot.settle(stdout)
}

// take hands the relay slot's verdict, once.
func (d *Desk) take(slot *Slot) ([]byte, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.claimableLocked(slot) {
		return nil, false
	}
	d.forgetLocked(slot)
	return slot.stdout, true
}

func (d *Desk) claimableLocked(slot *Slot) bool {
	return !slot.spent &&
		!slot.decidedAt.IsZero() &&
		time.Since(slot.decidedAt) < d.retention
}
