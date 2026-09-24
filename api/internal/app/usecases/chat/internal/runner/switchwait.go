package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// What a provider switch PARKS ON before it destroys anything, and what
// releases it. Both waits sit ahead of every read switchProviderLocked makes,
// so a parked switch holds nothing but its chat's spawn gate — see that
// function's own doc.

// forceSwitchAfter bounds how long a switch waits for the outgoing turn before
// forcing it. AwaitTurnComplete itself is documented as needing no timeout,
// because a live CLI either finishes its turn or eventually dies, and death is
// what reconcileRunnerExit turns into the same release signal. That reasoning
// has a gap: a CLI that is alive but stuck — no turn_stop, no exit, no delivery
// of any kind ever again — satisfies neither condition, and left the switch (and
// the "Starting <provider>…" spinner it drives) waiting forever. DefaultStallQuiet
// is reused rather than a second invented number: it is already this codebase's
// definition of "quiet long enough to call it stuck" (see termwait).
func (rs *Runners) forceSwitchAfter() time.Duration {
	if rs.switchAwaitTimeout > 0 {
		return rs.switchAwaitTimeout
	}
	return termwait.DefaultStallQuiet
}

// SetSwitchAwaitTimeout overrides the bound on BOTH waits a switch can park on
// — forceSwitchAfter's and promptDeliveryAwait's. Test-only surface —
// production never calls it — so a deterministic test can force either timeout
// path without actually waiting the real duration.
func (rs *Runners) SetSwitchAwaitTimeout(d time.Duration) { rs.switchAwaitTimeout = d }

// WaitingForPromptDeliveryLog is emitted at the INSTANT a switch parks on a
// committed-but-unconfirmed prompt delivery, so a test can block on a real
// signal rather than on a clock. Same role as turn.WaitingForTurnLog.
const WaitingForPromptDeliveryLog = "agent: switch provider: waiting for the prompt delivery already in flight"

// promptDeliveryAwait bounds how long a switch parks on a delivery Crowbar has
// COMMITTED but the provider has not yet confirmed.
//
// DefaultDeliveryQuiet, not forceSwitchAfter's DefaultStallQuiet: it is already
// this codebase's definition of how long an unconfirmed delivery may stay
// unconfirmed — the terminal-wait sweep retires one at exactly this age
// (SettleDelivery) — so waiting longer would only be waiting on a record that
// sweep has already settled.
func (rs *Runners) promptDeliveryAwait() time.Duration {
	if rs.switchAwaitTimeout > 0 {
		return rs.switchAwaitTimeout
	}
	return termwait.DefaultDeliveryQuiet
}

// awaitPromptDeliverySettled parks until requireNoPendingPromptDelivery stops
// answering "busy", and only then lets the switch proceed.
//
// The refusal it replaces was correct but unobservable, and that combination is
// the bug (measured live, twice): SubmitPrompt returns 200 the moment the
// replacement CLI is forked with the prompt in its argv, but the chat does not
// read Working until the provider's own user_prompt hook lands — ~1.5s later in
// both captures. A client that waits for `working == false` before switching
// therefore switches INSIDE that window and was answered 409 in about a
// millisecond, on a chat whose every visible signal said idle. Seconds later
// the same request succeeded, because the hook had since advanced the journal.
//
// Nothing is loosened: the switch still displaces nothing while a delivery is
// in flight, and a delivery still pending at the bound is still refused. The
// only change is that a delivery which resolves is waited for, exactly as the
// turn it is about to become is waited for.
func (rs *Runners) awaitPromptDeliverySettled(ctx context.Context, chat domain.Chat) error {
	deadline := time.After(rs.promptDeliveryAwait())
	logged := false
	for {
		// Both signals are taken BEFORE the journal is read, so a hook landing
		// between the read and the park below wakes this rather than being missed.
		_, turnChanged := rs.inflightTurns.Watch(chat.ID)
		_, _, workChanged := rs.work.Observe(chat.ID)
		err := rs.requireNoPendingPromptDelivery(ctx, chat)
		if !errors.Is(err, ErrPromptBusy) {
			return err
		}
		if !logged {
			slog.InfoContext(ctx, WaitingForPromptDeliveryLog, "chat_id", chat.ID)
			logged = true
		}
		select {
		case <-turnChanged:
		case <-workChanged:
		case <-deadline:
			// Re-asked rather than reported from the stale read above: a refusal
			// must name a delivery that is pending NOW.
			return rs.requireNoPendingPromptDelivery(ctx, chat)
		case <-ctx.Done():
			return fmt.Errorf("agent: switch provider: waiting for the prompt delivery in flight: %w", ctx.Err())
		}
	}
}

// awaitTurnOrForce is AwaitTurnComplete bounded by forceSwitchAfter. Distinguishing
// "my own added deadline fired" from "the CALLER's context died" matters:
// TestSwitchProvider_MidTurn_ContextCancelled_AbortsWithNothingChanged requires the
// latter to abort the switch with nothing touched, exactly as before this existed.
// ctx here is the CALLER's, unwrapped — only when it is still alive can the failure
// belong to the timeout this function added.
func (rs *Runners) awaitTurnOrForce(ctx context.Context, chatID string) error {
	bounded, cancel := context.WithTimeout(ctx, rs.forceSwitchAfter())
	defer cancel()
	err := rs.turns.AwaitTurnComplete(bounded, chatID)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil || !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return rs.forceOutgoingTurn(ctx, chatID)
}

// forceOutgoingTurn ends the outgoing turn the same way an explicit Stop click
// would (see StopChat): record the interruption, then tear the runner down.
// Always a full retire, never interruptTurn's gentler in-place cancel — by the
// time this runs, the CLI has already been given forceSwitchAfter to wrap up
// gracefully and has not, so a second, equally-graceful signal is not owed
// another wait for it to be silently ignored a second time. Accepting the small
// risk StopChat already accepts every day (a CLI terminated mid-turn may not
// flush its native transcript) beats a switch that never completes at all.
func (rs *Runners) forceOutgoingTurn(ctx context.Context, chatID string) error {
	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil // it finished or died between the deadline firing and this read
	}
	if err != nil {
		return fmt.Errorf("agent: switch provider: force outgoing turn: live runner: %w", err)
	}
	slog.WarnContext(ctx, "agent: switch provider: outgoing turn did not finish within the grace period; forcing it",
		"chat_id", chatID, "runner_id", live.ID, "waited", rs.forceSwitchAfter())
	rs.retire(ctx, live)
	// Recorded AFTER retire's kill, not before — see StopChat's own RecordStop
	// call for why: it must not durably claim "Interrupted" until the CLI has
	// actually stopped, and retire's kill is what makes that true here.
	if err := rs.turns.RecordStop(ctx, chatID, live.ID); err != nil {
		slog.WarnContext(ctx, "agent: switch provider: force outgoing turn: record interruption",
			"chat_id", chatID, "err", err)
	}
	return nil
}
