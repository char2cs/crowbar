package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// What a provider switch PARKS ON before it destroys anything, and what
// releases it. Both waits sit ahead of every read switchProviderLocked makes,
// so a parked switch holds nothing but its chat's spawn gate — see that
// function's own doc.

// WaitingForPendingPromptLog is logged the instant a switch parks on a prompt
// the provider has not acknowledged, so a test can block on a real signal.
const WaitingForPendingPromptLog = "agent: switch provider: waiting for the pending prompt"

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

// SetSwitchAwaitTimeout overrides forceSwitchAfter's bound. Test-only surface —
// production never calls it — so a deterministic test can force the timeout
// path without actually waiting the real duration.
func (rs *Runners) SetSwitchAwaitTimeout(d time.Duration) { rs.switchAwaitTimeout = d }

// awaitNoPendingPrompt parks until the chat holds no prompt the provider has
// yet to acknowledge: quitting the CLI carrying one would lose it. Only the
// ledger's next commit to the chat, or the end of park, releases it.
func (rs *Runners) awaitNoPendingPrompt(park context.Context, chatID string) error {
	logged := false
	for {
		if _, err := rs.settleOrphanedPrompts(park, chatID); err != nil {
			return fmt.Errorf("agent: switch provider: %w", err)
		}
		st, changed, err := rs.ledger.Watch(park, chatID)
		if err != nil {
			return fmt.Errorf("agent: switch provider: watch ledger: %w", err)
		}
		if len(st.PendingPrompts()) == 0 {
			return nil
		}
		if !logged {
			slog.InfoContext(park, WaitingForPendingPromptLog, "chat_id", chatID)
			logged = true
		}
		select {
		case <-changed:
		case <-park.Done():
			return fmt.Errorf("agent: switch provider: waiting for the pending prompt: %w", park.Err())
		}
	}
}

// awaitTurnOrForce is AwaitTurnComplete bounded by forceSwitchAfter. Distinguishing
// "my own added deadline fired" from "the CALLER's context died" matters:
// TestSwitchProvider_MidTurn_ContextCancelled_AbortsWithNothingChanged requires the
// latter to abort the switch with nothing touched, exactly as before this existed.
// The wait parks on park (the caller's context, also cancelled when Stop
// preempts the gate) — only while that is still alive can the failure belong to
// the timeout this function added. The forced teardown runs on ctx, so a Stop
// arriving mid-teardown cannot cut it in half.
func (rs *Runners) awaitTurnOrForce(ctx, park context.Context, chatID string) error {
	bounded, cancel := context.WithTimeout(park, rs.forceSwitchAfter())
	defer cancel()
	err := rs.turns.AwaitTurnComplete(bounded, chatID)
	if err == nil {
		return nil
	}
	if park.Err() != nil || !errors.Is(err, context.DeadlineExceeded) {
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
	rs.stopRunner(ctx, chatID, live, false)
	return nil
}
