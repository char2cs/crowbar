package answerdesk

import (
	"context"
	"time"
)

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
	timer := time.NewTimer(Wait(slot.Keys.Wait))
	defer timer.Stop()
	select {
	case <-slot.done:
		stdout, _ := d.take(slot)
		return HookAnswer{Stdout: stdout}, nil
	case <-timer.C:
		// The budget ran out and a verdict may have landed in the same instant;
		// claim before releasing, or a decision the user already made is thrown away.
		if stdout, claimed := d.take(slot); claimed {
			return HookAnswer{Stdout: stdout}, nil
		}
		if d.release(slot) {
			d.record(context.WithoutCancel(ctx), slot, onExpiry, "agent: answer: budget ran out")
		}
		return HookAnswer{}, nil
	case <-ctx.Done():
		return HookAnswer{}, ctx.Err()
	}
}

// Answer hands stdout to slot's relay once record reports the answer recorded.
func (d *Desk) Answer(
	ctx context.Context, slot *Slot, stdout []byte, record func(context.Context) (bool, error),
) (bool, error) {
	recorded, err := record(ctx)
	if err != nil || !recorded {
		return recorded, err
	}
	d.resolve(slot, stdout)
	return true, nil
}

// resolve records a verdict and wakes the relay. The prompt stops being
// answerable at once, but the slot lingers for the desk's retention so a relay
// that has not asked yet still finds its answer.
func (d *Desk) resolve(slot *Slot, stdout []byte) {
	defer d.announce(slot.ChatID)
	d.mu.Lock()
	defer d.mu.Unlock()
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

// release frees slot's relay unanswered, and reports whether it did: a verdict
// that landed meanwhile keeps it.
func (d *Desk) release(slot *Slot) bool {
	defer d.announce(slot.ChatID)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.claimableLocked(slot) {
		return false
	}
	d.forgetLocked(slot)
	slot.settle(nil)
	return true
}

func (d *Desk) claimableLocked(slot *Slot) bool {
	return !slot.spent &&
		!slot.decidedAt.IsZero() &&
		time.Since(slot.decidedAt) < d.retention
}
