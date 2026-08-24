package chat_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSwitchProvider_BackgroundWork_WaitsForAuthoritativeIdle proves a provider
// switch cannot treat turn_stop as "the CLI is done" when that same hook reports
// work still running. There is no sleep or projection polling: the switch announces
// that it is parked, and only the later authoritative zero-level hook releases it.
func TestSwitchProvider_BackgroundWork_WaitsForAuthoritativeIdle(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	f.announce(t, runnerID, "s1")
	prompt(t, f, runnerID, "claude", "launch a background subagent")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched.", 1)))
	f.wait()
	require.True(t, f.chat(t, chatID).Working, "precondition: async work keeps the chat live")

	killed := terminateSignal(f)
	parked := parkedOnTurn(t)
	done := make(chan switchResult, 1)
	go func() {
		id, err := f.usecase.SwitchProvider(context.Background(), chatID, "codex")
		done <- switchResult{runnerID: id, err: err}
	}()

	select {
	case <-parked:
	case sess := <-killed:
		t.Fatalf("the outgoing CLI (%s) was terminated while its background work was live", sess)
	case got := <-done:
		t.Fatalf("the switch returned while background work was live: %+v", got)
	}
	require.Empty(t, f.term.terminatedIDs(), "background work must keep the outgoing TUI alive")

	// Claude's later status hook restates the level at zero. Only this semantic
	// transition — not elapsed time and not projection convergence — may release it.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "The background subagent finished.", 0)))

	got := <-done
	require.NoError(t, got.err)
	f.wait()
	require.Contains(t, f.term.terminatedIDs(), oldTerm)
}

// stopPayload is a real claude 2.1.212 Stop hook payload carrying `running` background
// tasks — the shape traced live while a background subagent was working. tasks is how
// many entries claude reports STILL OUTSTANDING as it goes quiet.
func stopPayload(t *testing.T, message string, tasks int) []byte {
	t.Helper()
	bg := make([]any, 0, tasks)
	for range tasks {
		bg = append(bg, map[string]any{
			"id":         "abbe4333c2384e2dc",
			"type":       "subagent",
			"status":     "running",
			"agent_type": "general-purpose",
		})
	}
	return mustJSON(t, map[string]any{
		"session_id":             "s1",
		"last_assistant_message": message,
		"background_tasks":       bg,
		"session_crons":          []any{},
	})
}

// TestRegression_TurnStopWithBackgroundSubagent_KeepsChatWorking is THE BUG, end to end
// through the real usecase, the real descriptor and the real aggregate — the hook payload
// in, the spinner out.
//
// Traced against claude 2.1.212: the CLI spawns a BACKGROUND subagent, then goes quiet
// waiting to be re-invoked when it reports back — which ends its turn for real, and it
// fires Stop right there, ~18 seconds before the subagent actually finished. Crowbar read
// that Stop as "done" and darkened the spinner on a chat whose agent was still working.
// The user thinks it died.
//
// This is the guard on the wiring between them: the level claude reports on its Stop must
// travel from the hook payload into the fold. Dropping it on the floor in the usecase —
// passing 0 instead of ev.AsyncWork — reproduces the original bug exactly, with every
// other test still green.
func TestRegression_TurnStopWithBackgroundSubagent_KeepsChatWorking(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "launch a background subagent and wait for it"})))
	require.True(t, f.chat(t, chatID).Working, "precondition: the turn is open")

	// claude hands the work off and ends its turn — with one subagent still running.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched. The subagent is running in the background.", 1)))
	f.wait()

	chat := f.chat(t, chatID)
	require.True(t, chat.Working,
		"the spinner must KEEP SPINNING: the turn ended but a background subagent is still working")
	require.Nil(t, chat.CurrentTurnStarted, "the turn itself really did end")
	require.Equal(t, 1, chat.AsyncWork)
}

// TestRegression_BackgroundSubagentFinishes_StopsChatWorking is the other half, and the
// one that keeps the fix from becoming a WORSE bug than the one it fixes: the spinner has
// to actually STOP. A permanently-spinning spinner lies forever, and this is an
// event-sourced aggregate, so it would survive restarts.
//
// Traced: when the subagent reports back claude re-invokes itself (a UserPromptSubmit
// carrying a <task-notification>), answers, and ends THAT turn with background_tasks: [].
func TestRegression_BackgroundSubagentFinishes_StopsChatWorking(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "launch a background subagent"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched.", 1)))
	f.wait()
	require.True(t, f.chat(t, chatID).Working, "precondition: spinning on the subagent")

	// The subagent reports back: claude re-invokes itself and ends the turn with nothing left.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "<task-notification><task-id>abbe4333c2384e2dc</task-id>"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "The subagent finished.", 0)))
	f.wait()

	chat := f.chat(t, chatID)
	require.False(t, chat.Working, "once the work is done the spinner MUST stop — no stuck-on")
	require.Equal(t, 0, chat.AsyncWork)
}

// TestRegression_ConcurrentBackgroundSubagentsDrain_StopsChatWorking is the traced
// multi-subagent case: three at once, and claude RESTATES the whole list on every Stop as
// it drains ([running x3] → [running x4] → [running] → []). The level follows it down and
// lands idle — no pairing, no arithmetic, nothing to leak.
func TestRegression_ConcurrentBackgroundSubagentsDrain_StopsChatWorking(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	// Each restatement claude actually emitted, in order. A subagent spawning MORE
	// subagents (3 → 4) is why this must never be a decrementing counter.
	for _, outstanding := range []int{3, 4, 1, 0} {
		require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
			mustJSON(t, map[string]any{"prompt": "turn"})))
		require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
			stopPayload(t, "restated", outstanding)))
		f.wait()

		if outstanding > 0 {
			require.Truef(t, f.chat(t, chatID).Working,
				"%d subagents still running: must keep spinning", outstanding)
		}
	}

	require.False(t, f.chat(t, chatID).Working, "drained to zero: the spinner must stop")
}

// TestRegression_InterruptedTurnThenNewPrompt_DoesNotStrandSpinner is the case the
// PREVIOUS attempt broke, and it is pre-existing in shape.
//
// Traced: an INTERRUPT (ESC) fires NO HOOK AT ALL — not Stop, not Notification, nothing —
// so the turn it interrupted is never closed by the CLI, and any async work it announced
// is never retired. The previous attempt counted work_begin/work_end edges, so an
// interrupt during background work stranded the count at 3 and spun that chat FOREVER.
//
// Here the next prompt supersedes both: a new turn zeroes the level, and that turn's own
// Stop settles it. The spinner comes back to the truth.
func TestRegression_InterruptedTurnThenNewPrompt_DoesNotStrandSpinner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	// A turn that ended with background work outstanding...
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "launch three background subagents"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched three.", 3)))
	f.wait()
	require.True(t, f.chat(t, chatID).Working, "precondition: spinning on 3 subagents")

	// ...the user hits ESC. NOTHING arrives — that is the whole point; no hook exists.
	// Then they type again. This is the only edge that can heal it.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "hi"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "hello", 0)))
	f.wait()

	chat := f.chat(t, chatID)
	require.False(t, chat.Working,
		"an interrupt must not strand the spinner: the next completed turn settles it")
	require.Equal(t, 0, chat.AsyncWork)
}

// TestRegression_KilledCLIWithBackgroundWork_DoesNotSpinForever is the stuck-on case that
// the hook surface cannot fix, and the reconcile must.
//
// Traced: SIGKILL mid-background-work sends NO SessionEnd and NO final Stop. The last word
// on the aggregate is a turn_stop reporting work still running, with nobody left alive to
// restate it — and in an event-sourced aggregate that word outlives the daemon. The boot
// reconcile (a dead PTY cannot still be working) is what clears it.
func TestRegression_KilledCLIWithBackgroundWork_DoesNotSpinForever(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "launch a background subagent"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched.", 1)))
	f.wait()
	require.True(t, f.chat(t, chatID).Working, "precondition: spinning on announced work")
	require.Nil(t, f.chat(t, chatID).CurrentTurnStarted,
		"and NOT because a turn is open — the turn closed; only the work keeps it lit")

	// The CLI dies with the daemon. No Stop is coming, ever.
	f.term.dieWithDaemon()
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	chat := f.chat(t, chatID)
	require.False(t, chat.Working,
		"a dead CLI's announced work must not spin the chat forever across a restart")
	require.Equal(t, 0, chat.AsyncWork)
}

// TestCodexTurnStop_NeverReportsAsyncWork is the PROVIDER-AGNOSTIC requirement through the
// live usecase: codex maps no async_work, so even a Stop payload that happens to carry a
// background_tasks array leaves it turn-only and bit-identical to before this existed.
func TestCodexTurnStop_NeverReportsAsyncWork(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "do a thing"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "turn_stop",
		stopPayload(t, "done", 3)))
	f.wait()

	chat := f.chat(t, chatID)
	require.False(t, chat.Working, "codex maps no async_work: turn_stop is simply idle")
	require.Equal(t, 0, chat.AsyncWork, "an unmapped field must never be counted")
}
