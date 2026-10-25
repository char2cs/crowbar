package answerdesk

import (
	"context"
	"fmt"
	"log/slog"
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
		return nil, slot.answering.done, false
	case slot.spent:
		return nil, nil, false
	}
	d.forgetLocked(slot)
	slot.settle(nil)
	return nil, nil, true
}

// Record writes an answer to the ledger and reports whether it did. It writes
// only while waiting reports a relay still waiting on the answer.
type Record func(ctx context.Context, waiting func() bool) (bool, error)

// Answer hands stdout to the relay holding slot's ask if record, run while that
// relay is claimed for this answer alone, reports it recorded. An answer no relay
// waits on any more is refused with ErrNotWaiting; a failed record hands it back.
func (d *Desk) Answer(ctx context.Context, slot *Slot, stdout []byte, record Record) (bool, error) {
	c, ok := d.claim(slot)
	if !ok {
		return false, ErrNotWaiting
	}
	recorded, err := record(ctx, func() bool { return d.waiting(c) })
	if err == nil && recorded {
		d.resolve(ctx, c, stdout)
		return true, nil
	}
	if waited := d.unclaim(c); err == nil && !waited {
		return false, ErrNotWaiting
	}
	return recorded, err
}

// claim claims the relay holding slot's ask now: a re-ask may have displaced
// slot since the answer looked it up.
func (d *Desk) claim(slot *Slot) (*claim, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if current, held := d.slots[slot.ChoiceID]; held {
		slot = current
	}
	if slot.spent || !slot.decidedAt.IsZero() || slot.answering != nil {
		return nil, false
	}
	slot.answering = &claim{slot: slot, done: make(chan struct{})}
	return slot.answering, true
}

// waiting reports whether a relay still waits on c's answer.
func (d *Desk) waiting(c *claim) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !c.slot.spent
}

// unclaim hands c's relay back to its budget, and reports whether one waits.
func (d *Desk) unclaim(c *claim) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.endClaimLocked(c)
	return !c.slot.spent
}

func (d *Desk) endClaimLocked(c *claim) {
	if c.slot.answering == c {
		c.slot.answering = nil
	}
	close(c.done)
}

// resolve records c's verdict and wakes the relay holding its ask now. The
// prompt stops being answerable at once, but the slot lingers for the desk's
// retention so a relay that has not asked yet still finds its answer.
func (d *Desk) resolve(ctx context.Context, c *claim, stdout []byte) {
	d.mu.Lock()
	slot := c.slot
	d.endClaimLocked(c)
	handed := !slot.spent && slot.decidedAt.IsZero()
	if handed {
		slot.decidedAt = time.Now()
		slot.reap = time.AfterFunc(d.retention, d.dropExpired)
		slot.settle(stdout)
	}
	d.mu.Unlock()
	if !handed {
		// Its relay was released after the record checked it waited.
		slog.WarnContext(ctx, "agent: answer: recorded, but no relay waits on it",
			"chat_id", slot.ChatID, "choice_id", slot.ChoiceID)
	}
	d.announce(slot.ChatID)
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
