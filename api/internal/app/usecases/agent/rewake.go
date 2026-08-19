package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
)

// The rewake delivery channel, daemon side.
//
// It is a PULL. The daemon never writes into a provider process; a collector the
// provider itself started — one `crowbar hook await-prompt` per armed hook —
// blocks here until there is a message for the chat its runner is on, and takes
// it. That inversion is what makes the outcome knowable: Crowbar does not have to
// guess whether a delivery landed, because nothing leaves this file until
// somebody has taken it.
//
// EVERY PROPERTY BELOW EXISTS TO PROTECT ONE SENTENCE: a message the user typed
// is never lost. Losing it is worse than restarting the CLI, worse than a slow
// delivery, and worse than an error the user can see and retry. So:
//
//   - The handoff is an UNBUFFERED channel send. Go pairs a send with a receive
//     atomically, so `handed=false` is proof — not an assumption — that no
//     collector has these bytes, and the caller may fall back to a restart
//     without any risk of delivering the same message twice.
//   - "Is the channel available" is never inferred from a descriptor, a timer or
//     a heartbeat. It is the physical presence of a blocked collector, so a
//     provider that lost its hook (its timeout expired, it was never armed, the
//     release withdrew the flag) is detected instantly and restarts instead.
//   - Plaintext lives HERE and nowhere else: in memory, for as long as the
//     handoff takes, never on disk and never in a log. The durable journal keeps
//     the SHA-256 it always kept.
const (
	// rewakeHandoffBudget bounds the wait for a collector to take a message.
	//
	// It is short on purpose. A collector is already blocked and waiting when this
	// starts, so the handoff is a channel send between two goroutines in one
	// process; the only thing this budget is really waiting out is the microscopic
	// race where the collector's own deadline fires first. Anything longer would
	// be a user watching a spinner before Crowbar decides to restart after all.
	rewakeHandoffBudget = 2 * time.Second

	// rewakeAckBudget bounds the wait for the collector to confirm it has WRITTEN
	// the message back to its process. Past the handoff there is nothing to undo,
	// so this budget only decides whether the outcome is reported as delivered or
	// as unknown — never whether it is retried.
	rewakeAckBudget = 5 * time.Second

	// rewakeMinPoll and rewakeMaxPoll bound how long ONE collect blocks.
	//
	// The ceiling is the daemon's, not the collector's, and it is deliberately not
	// hours: a collector resolves which chat its runner is on when it joins, and a
	// CLI that changes conversation on its own (a /clear, a native /resume) makes
	// that answer stale. Ending the poll is how that answer gets re-taken, so the
	// ceiling is also the bound on how stale it can be.
	rewakeMinPoll = time.Second
	rewakeMaxPoll = 60 * time.Second
)

// errRewakeNoChannel is returned to a collector whose runner can no longer
// receive anything — it is gone, or it has been displaced off every chat. It is
// the signal that ends the collector for good rather than sending it round
// again, and it is what reaps the process claude leaves behind: collectors are
// spawned DETACHED, so the CLI dying does not kill them, and this is the only
// thing that does.
var errRewakeNoChannel = errors.New("agent: rewake: this runner has no prompt channel")

// rewakeHandoff is one message in flight between the submitting request and the
// collector that takes it.
type rewakeHandoff struct {
	text string
	// ack is closed by the collector once the message is on its way back to the
	// provider. It cannot un-deliver anything; it exists so the submitter can tell
	// "delivered" from "handed over and then unheard of", and report the second as
	// unknown rather than as success.
	ack     chan struct{}
	ackOnce sync.Once
}

// rewakeSlot is one runner's collectors. They share a single handoff channel, so
// a provider that armed more than one collector still takes each message exactly
// once — the unbuffered send has exactly one receiver.
type rewakeSlot struct {
	// chatID is the chat the MOST RECENT collector resolved onto. A message is
	// only offered to a slot whose chat matches, which is what stops a prompt
	// following a runner that has since moved to a different conversation.
	chatID  string
	waiting int
	handoff chan *rewakeHandoff
}

// rewakeDesk is the registry of blocked collectors. It holds no queue: a message
// that nobody is waiting for is not parked, it is refused, and the caller
// restarts the CLI instead. A queue would be a place for a prompt to sit and be
// forgotten.
type rewakeDesk struct {
	mu    sync.Mutex
	slots map[string]*rewakeSlot

	// onJoin, when set, fires once a collector is registered and can be handed a
	// message. It exists so a test can act on a collector being PROVABLY blocked
	// rather than on a poll that thinks it probably is — the difference between a
	// causal signal and a race with a friendly name. Nil in production.
	onJoin func(runnerID string)
}

func newRewakeDesk() *rewakeDesk {
	return &rewakeDesk{slots: map[string]*rewakeSlot{}}
}

// join registers a collector for runnerID on chatID and returns the channel it
// must receive on, plus the function that unregisters it.
func (d *rewakeDesk) join(runnerID, chatID string) (<-chan *rewakeHandoff, func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	slot := d.slots[runnerID]
	if slot == nil {
		slot = &rewakeSlot{handoff: make(chan *rewakeHandoff)}
		d.slots[runnerID] = slot
	}
	slot.chatID = chatID
	slot.waiting++
	ch := slot.handoff
	if d.onJoin != nil {
		// Announced while the registration is still under the lock, so a test woken
		// by it can never observe a desk that has not finished registering.
		d.onJoin(runnerID)
	}

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			slot.waiting--
			if slot.waiting <= 0 && d.slots[runnerID] == slot {
				delete(d.slots, runnerID)
			}
		})
	}
}

// deliver offers text to a collector blocked for this runner on this chat.
//
// handed reports whether a collector TOOK the bytes, and it is the only value a
// caller may act on destructively: false means nothing left this process, so the
// message can still be delivered by restarting the CLI. acked reports whether
// that collector then confirmed it wrote them back; it is diagnosis, never
// permission to retry.
func (d *rewakeDesk) deliver(
	runnerID, chatID, text string,
	handoffBudget, ackBudget time.Duration,
) (handed, acked bool) {
	d.mu.Lock()
	slot := d.slots[runnerID]
	// The chat check is under the same lock as the read, so a collector that
	// joined for another conversation can never be handed this one's message.
	if slot == nil || slot.waiting == 0 || slot.chatID != chatID {
		d.mu.Unlock()
		return false, false
	}
	ch := slot.handoff
	d.mu.Unlock()

	message := &rewakeHandoff{text: text, ack: make(chan struct{})}
	handoff := time.NewTimer(handoffBudget)
	defer handoff.Stop()
	select {
	case ch <- message:
	case <-handoff.C:
		// Every collector left between the read above and this send. Nothing was
		// handed over, so the caller is free to restart.
		return false, false
	}

	ack := time.NewTimer(ackBudget)
	defer ack.Stop()
	select {
	case <-message.ack:
		return true, true
	case <-ack.C:
		return true, false
	}
}

// waiting reports how many collectors are blocked for runnerID. It exists for
// tests and for a log line; no delivery decision is taken from it, because a
// count read a moment before a send proves nothing that the send does not prove
// better.
func (d *rewakeDesk) waiting(runnerID string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if slot := d.slots[runnerID]; slot != nil {
		return slot.waiting
	}
	return 0
}

// collect blocks until this runner's chat has a message, the budget runs out, or
// the caller goes away.
func (d *rewakeDesk) collect(
	ctx context.Context,
	runnerID, chatID string,
	budget time.Duration,
) *rewakeHandoff {
	ch, leave := d.join(runnerID, chatID)
	defer leave()

	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case message := <-ch:
		// Taken. From here this collector is committed: nothing else can receive
		// this message, and its caller is obliged to write it out and then ack.
		return message
	case <-ctx.Done():
		return nil
	case <-timer.C:
		return nil
	}
}

// deliveredAck reports that a taken message reached its process. It is idempotent
// and safe on a nil handoff, so a caller may defer it unconditionally.
func (h *rewakeHandoff) delivered() {
	if h == nil {
		return
	}
	h.ackOnce.Do(func() { close(h.ack) })
}

// AwaitQueuedPrompt is the collector's whole API: block until this runner's
// current chat has a message for it, and hand back what the user typed.
//
// IT IS THE ONE CROWBAR CALLBACK THAT READS. Every other one writes into the
// daemon and is content with a segment id, which proves nothing — segment ids are
// published on the chats API. This one returns a person's words, so it carries
// the same per-boot HMAC the MCP surface uses. That is not containment against an
// agent with a shell, and it is not meant to be: argv is world-readable on this
// machine either way. What it buys is that reading another runner's prompts
// cannot happen by ACCIDENT, and that a collector holds nothing that outlives the
// daemon that minted it.
//
// ok=false with a nil error means "nothing yet, ask again". An error means "stop
// asking": either the credential is wrong, or the runner this collector belongs
// to is gone.
// ack is never nil, so a caller may defer it whatever the outcome, and it must be
// called only once the bytes are on their way back — it is the daemon's own
// record that this delivery completed.
func (u *Usecase) AwaitQueuedPrompt(
	ctx context.Context,
	runnerID, token string,
	waitMS int64,
) (text string, ok bool, ack func(), err error) {
	chatID, err := u.rewakeChannel(ctx, runnerID, token)
	if err != nil {
		return "", false, func() {}, err
	}
	message := u.rewake.collect(ctx, runnerID, chatID, clampRewakePoll(waitMS))
	if message == nil {
		return "", false, func() {}, nil
	}
	return message.text, true, message.delivered, nil
}

// rewakeChannel authenticates a collector and answers which conversation it is
// currently collecting for.
//
// It re-reads the runner on EVERY poll rather than trusting what the collector
// was told at spawn, because a CLI moves between chats on its own — a /clear, a
// native /resume — and a collector that kept its original answer would carry one
// chat's message into another. Nothing here is provider-specific: it is the same
// "resolve the runner's CURRENT chat" rule the MCP tool surface resolves by.
func (u *Usecase) rewakeChannel(ctx context.Context, runnerID, token string) (string, error) {
	// Fail CLOSED on a daemon with no minter. Without this the verify below would
	// panic, and a misconfiguration must cost a feature, never the process.
	if u.minter == nil {
		return "", fmt.Errorf("agent: await prompt: no token minter: %w", agenttools.ErrUnauthorized)
	}
	if runnerID == "" || !u.minter.Verify(runnerID, token) {
		return "", fmt.Errorf("agent: await prompt: %w", agenttools.ErrUnauthorized)
	}
	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		// A runner the store has never heard of, or has forgotten, is a collector
		// with nothing left to collect for. Reported as no-channel rather than as a
		// transient failure, so the process ends instead of polling a dead id
		// forever — claude spawns it detached, so nothing else will end it.
		return "", fmt.Errorf("agent: await prompt: %w: %w", errRewakeNoChannel, err)
	}
	if runner.CurrentChatID == "" {
		// Displaced: evicted from its chat, dying but not yet reaped.
		return "", fmt.Errorf("agent: await prompt: runner is on no chat: %w", errRewakeNoChannel)
	}
	return runner.CurrentChatID, nil
}

// clampRewakePoll holds a collector's requested budget inside the daemon's own
// bounds. The ceiling matters: the daemon must be the one that ends the poll, so
// the collector exits under its own control instead of being cut off by the
// provider's hook timeout mid-write.
func clampRewakePoll(waitMS int64) time.Duration {
	wait := time.Duration(waitMS) * time.Millisecond
	if wait < rewakeMinPoll {
		return rewakeMinPoll
	}
	if wait > rewakeMaxPoll {
		return rewakeMaxPoll
	}
	return wait
}
