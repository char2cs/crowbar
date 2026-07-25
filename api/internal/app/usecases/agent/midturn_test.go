package agent_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
)

// switchResult is what a SwitchProvider driven on its own goroutine hands back.
type switchResult struct {
	runnerID string
	err      error
}

// TestSwitchProvider_MidTurn_WaitsForTheTurnBeforeQuittingTheOutgoingCLI is the headline
// regression, and it is a bug the user hit in production:
//
//	claude --resume → "No conversation found with session ID: <id>"
//
// A switch used to SIGTERM the outgoing CLI the instant it was asked to. A SIGTERM is
// what it is (rather than a SIGKILL) precisely so the CLI can flush its native
// transcript on the way out — but a CLI killed MID-TURN never writes a transcript at
// all. So the in-flight answer was lost, and the conversation the incoming CLI was then
// told to --resume did not exist.
//
// The fix is to let the turn finish first. This test proves it WITHOUT ANY TIMING: the
// switch runs on its own goroutine and the test blocks on whichever of two REAL signals
// arrives —
//
//	the switch announcing it is parked on the turn (the fix), or
//	TerminateGraceful being called (the bug, caught in the act, inside the fake).
//
// Exactly one of them must arrive, so there is nothing to sleep for and nothing to poll.
func TestSwitchProvider_MidTurn_WaitsForTheTurnBeforeQuittingTheOutgoingCLI(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	f.announce(t, runnerID, "sid-claude-native")

	// The user typed: the CLI is now mid-answer, and this is the state the whole bug
	// lives in.
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

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
		t.Fatalf("the outgoing CLI (%s) was terminated while its turn was still running: "+
			"its answer is lost and the transcript it never flushed cannot be --resume'd", sess)
	case r := <-done:
		t.Fatalf("the switch returned without ever reaching the outgoing CLI: %+v", r)
	}

	// Still nobody has been killed, and no incoming CLI has been spawned: the switch is
	// parked, holding its chat's spawn gate. The hook path is NEVER gated, which is what
	// lets the very next line reach the usecase at all — if it could not, this test would
	// deadlock, and that deadlock is the one this design has to be safe from.
	assert.Empty(t, f.term.terminatedIDs(), "nothing may be killed while the turn is in flight")
	assert.Equal(t, 1, f.term.callCount(), "and no incoming CLI may be spawned yet")

	// The turn completes.
	turn(t, f, runnerID, "claude", "the answer the user was waiting for")

	got := <-done
	require.NoError(t, got.err)
	f.wait()

	assert.Contains(t, f.term.terminatedIDs(), oldTerm, "and only THEN is the outgoing CLI quit")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, got.runnerID, live.ID, "the switch then proceeds")
	assert.Equal(t, "codex", live.ProviderID)

	// The other half of the bug: the in-flight answer must not be lost. The ledger is read
	// AFTER the turn landed in it, so the incoming CLI is handed the reply the outgoing one
	// was still writing when the user clicked switch.
	require.Equal(t, 2, f.term.callCount())
	assert.Contains(t, strings.Join(f.term.calls[1].argv, "\x00"), "the answer the user was waiting for",
		"the handoff must carry the turn the switch waited for")
}

// TestSwitchProvider_Idle_SwitchesImmediately: the common case. A chat that is not
// working has no turn to wait for, so the switch must not park — the happy path pays
// nothing for the mid-turn guarantee.
func TestSwitchProvider_Idle_SwitchesImmediately(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	prompt(t, f, runnerID, "claude", "a question")
	turn(t, f, runnerID, "claude", "an answer")
	require.False(t, f.chat(t, chatID).Working, "precondition: the chat is idle")

	parked := parkedOnTurn(t)

	// Straight-line: no goroutine, no signal to wait for. If the switch parked, this call
	// would never return.
	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	select {
	case <-parked:
		t.Fatal("an idle chat has no in-flight turn: the switch must not wait for one")
	default:
	}

	assert.Contains(t, f.term.terminatedIDs(), oldTerm)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID)
}

// TestSwitchProvider_MidTurn_ContextCancelled_AbortsWithNothingChanged: the wait is
// bounded by the CALLER'S CONTEXT and by nothing else — no timeout, no deadline, no
// clock. If the request goes away, the switch aborts exactly as every other pre-terminate
// failure aborts it: the outgoing CLI is still alive, still on its chat, still mid-turn,
// and no incoming CLI was ever spawned. Half-doing it is the one outcome that is not
// allowed.
func TestSwitchProvider_MidTurn_ContextCancelled_AbortsWithNothingChanged(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	ctx, cancel := context.WithCancel(context.Background())
	killed := terminateSignal(f)
	parked := parkedOnTurn(t)

	done := make(chan switchResult, 1)
	go func() {
		id, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
		done <- switchResult{runnerID: id, err: err}
	}()

	select {
	case <-parked:
	case sess := <-killed:
		cancel()
		t.Fatalf("the outgoing CLI (%s) was terminated mid-turn", sess)
	case r := <-done:
		cancel()
		t.Fatalf("the switch returned without ever reaching the outgoing CLI: %+v", r)
	}

	cancel()

	got := <-done
	require.Error(t, got.err)
	assert.ErrorIs(t, got.err, context.Canceled)
	assert.Empty(t, got.runnerID)

	f.wait()
	assert.Empty(t, f.term.terminatedIDs(), "an aborted switch kills nothing")
	assert.Equal(t, 1, f.term.callCount(), "and spawns nothing")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID, "the outgoing CLI is still on its chat")
	assert.True(t, f.chat(t, chatID).Working, "and still mid-turn")
}

// TestSwitchProvider_MidTurn_OutgoingCLIDies_ReleasesTheSwitch: the turn's completion is
// not the only way it can END. A CLI that falls over mid-answer never sends its turn_stop
// hook — so if the wait listened for that hook alone, a switch onto a crashed CLI would
// hang until the user's request timed out. The PTY's death is the other real signal, and
// it releases the wait too (the chat is simply dormant by the time we get there).
func TestSwitchProvider_MidTurn_OutgoingCLIDies_ReleasesTheSwitch(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	prompt(t, f, runnerID, "claude", "think hard about this")

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
		t.Fatalf("the outgoing CLI (%s) was terminated mid-turn", sess)
	case r := <-done:
		t.Fatalf("the switch returned without ever reaching the outgoing CLI: %+v", r)
	}

	// The CLI crashes mid-answer.
	f.term.exit(t, oldTerm)

	got := <-done
	require.NoError(t, got.err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, got.runnerID, live.ID)
	assert.Equal(t, "codex", live.ProviderID, "the switch completes onto the dead CLI's chat")
	assert.NotEqual(t, runnerID, live.ID)
}

// TestResumeChat_DormantChat_DoesNotWait: ResumeChat shares switchProviderLocked, and a
// dormant chat has no CLI and therefore no turn anybody could still be running — even
// though its Working flag may still be set (a chat whose CLI died mid-turn). The revive
// must not wait for a turn nothing is running.
func TestResumeChat_DormantChat_DoesNotWait(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	prompt(t, f, runnerID, "claude", "a question")
	turn(t, f, runnerID, "claude", "an answer")

	// It dies mid-turn: nothing is running, and nothing will ever close this turn.
	prompt(t, f, runnerID, "claude", "and one more thing")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	parked := parkedOnTurn(t)

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	select {
	case <-parked:
		t.Fatal("a dormant chat has nothing running: a revive must never wait for a turn")
	default:
	}

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "claude", live.ProviderID)
}

// prompt drives a user_prompt hook: the user submitting a message, which is what OPENS a
// turn (the chat goes Working) in production. Every mid-turn test starts here, because
// this hook is the only thing that ever puts a chat mid-turn.
func prompt(
	t *testing.T,
	f testFixture,
	runnerID, provider, message string,
) {
	t.Helper()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, provider, "user_prompt",
		mustJSON(t, map[string]any{"prompt": message})))
	f.wait()
}

// terminateSignal reports the id of the FIRST terminal session TerminateGraceful is
// called with, as it is called. A test blocks on it to catch a kill in the act.
func terminateSignal(
	f testFixture,
) <-chan string {
	ch := make(chan string, 1)
	f.term.duringTerminate = func(sessionID string) {
		select {
		case ch <- sessionID:
		default:
		}
	}
	return ch
}

// parkedOnTurn returns a channel closed the moment a switch parks on an in-flight turn.
// It is a REAL signal — the usecase's own log record, emitted immediately before it
// blocks — and it is what lets these tests prove a negative ("the CLI was not killed
// while the turn ran") without a single sleep: the assertion is made at a moment the test
// KNOWS the switch has reached, not one it hopes it has.
func parkedOnTurn(
	t *testing.T,
) <-chan struct{} {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := &parkHandler{ch: make(chan struct{})}
	slog.SetDefault(slog.New(h))
	return h.ch
}

type parkHandler struct {
	once sync.Once
	ch   chan struct{}
}

func (h *parkHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *parkHandler) Handle(_ context.Context, r slog.Record) error {
	if strings.Contains(r.Message, agentusecase.WaitingForTurnLog) {
		h.once.Do(func() { close(h.ch) })
	}
	return nil
}

func (h *parkHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *parkHandler) WithGroup(_ string) slog.Handler { return h }

// TestRegression_ClearMidTurn_ClosesTheTurnOnTheChatLeftBehind is the workspace-spinner
// wedge, reported from production as "all the agents stopped working but the workspace
// is still spinning".
//
// A turn is opened by the user_prompt hook and closed by the turn_stop hook, and BOTH
// are filed against the chat the runner is on WHEN THEY ARRIVE. So a /clear taken
// mid-turn splits the pair: user_prompt landed on the old chat, the runner then walks
// into a freshly minted one, and the turn_stop lands over there. Nothing closes the
// turn where it was opened.
//
// The cost is not cosmetic. domain.AgentChat.Working stays true forever, and the
// workspace's derived Working overlay (repositories.Container.agentWorking) keeps that
// chat in its mid-turn set for the life of the daemon — so the sidebar and context-pill
// spinners run forever over a workspace where nothing at all is happening. The chat ROWS
// look idle the whole time, because the frontend clears its own working map on every
// reseed while the daemon-side set has no such reset. That asymmetry is exactly what the
// bug report described.
//
// The runner has left, and no successor has taken the old chat, so closeAbandonedTurn's
// own guards are the whole judgement: it asserts nothing about any live process.
func TestRegression_ClearMidTurn_ClosesTheTurnOnTheChatLeftBehind(t *testing.T) {
	f := newFixture(t)

	left, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")

	// The user typed, and then hit /clear before the answer came back.
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, left).Working, "precondition: the chat is mid-turn")

	f.announce(t, runnerID, "sid-after-clear") // /clear: mints a chat, moves the runner
	f.wait()

	entered := f.runner(t, runnerID).CurrentChatID
	require.NotEqual(t, left, entered, "precondition: the /clear moved the runner off")

	assert.False(t, f.chat(t, left).Working,
		"the chat the runner walked out of must not be left mid-turn: its turn_stop is "+
			"going to land on the new chat, so nothing else will ever close it here")
	assert.Nil(t, f.chat(t, left).CurrentTurnStarted, "a closed turn must clear CurrentTurnStarted")
}
