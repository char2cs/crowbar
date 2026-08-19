package agent_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// collectorBudgetMS is the poll one test collector asks the daemon to hold for.
// No test waits it out: every one of them is released by a real event — a message
// handed over, or the test finishing and cancelling. It is a ceiling on a hang,
// never a delay anything waits for.
const collectorBudgetMS = 30_000

// rewakeWrapper builds the payload the vendor CLI hands its own user-prompt hook
// after collecting a message, exactly as claude 2.1.235 was MEASURED to build it
// on 2026-08-18 (captured from raw UserPromptSubmit stdin on a live interactive
// PTY). The two Crowbar-controlled halves are read out of the SHIPPED descriptor,
// so a descriptor whose sentinel drifted from its strip pattern fails here.
func rewakeWrapper(t *testing.T, f testFixture, text string) string {
	t.Helper()
	agent, err := f.engine.Get(context.Background(), f.ws.home, "claude")
	require.NoError(t, err)
	sentinel := engineagents.RewakeSentinel(agent)
	summary := engineagents.RewakeSummary(agent)
	require.NotEmpty(t, sentinel, "the shipped claude descriptor must declare a rewake sentinel")
	require.NotEmpty(t, summary)
	return "<task-notification>\n" +
		"<summary>" + summary + "</summary>\n" +
		"</task-notification>\n" +
		"<system-reminder>\n" +
		sentinel + " " + text + "\n" +
		"</system-reminder>"
}

// collector blocks one prompt collector against the daemon, exactly as the in-PTY
// `crowbar hook await-prompt` process does, and returns only once it is REGISTERED
// and can be handed a message.
//
// The readiness signal is the desk's own join callback rather than a poll: the
// property most of these tests turn on is which side of "a collector exists" the
// submission lands on, and a test that guessed that would sometimes prove the
// opposite of its name.
func (f testFixture) collector(t *testing.T, runnerID string) <-chan string {
	t.Helper()
	joined := make(chan struct{})
	var once sync.Once
	agentusecase.SetRewakeJoinHook(f.usecase, func(string) { once.Do(func() { close(joined) }) })
	t.Cleanup(func() { agentusecase.SetRewakeJoinHook(f.usecase, nil) })

	ctx, cancel := context.WithCancel(f.ctx)
	t.Cleanup(cancel)

	collected := make(chan string, 1)
	go func() {
		defer close(collected)
		text, ok, ack, err := f.usecase.AwaitQueuedPrompt(
			ctx, runnerID, f.minter.Mint(runnerID), collectorBudgetMS)
		if err != nil || !ok {
			return
		}
		// The relay writes the message out and only then acknowledges; this stands
		// in for that write.
		ack()
		collected <- text
	}()
	<-joined
	return collected
}

// turnEnds drives the completed turn a rewake needs before it can exist: the
// provider arms its collector when a turn stops.
func turnEnds(t *testing.T, f testFixture, runnerID, provider, reply string) {
	t.Helper()
	turn(t, f, runnerID, provider, reply)
}

// ---------------------------------------------------------------------------
// The trap. Both directions, on the shipped descriptor.
// ---------------------------------------------------------------------------

// TestRegression_RewakeDeliveredPromptIsRecordedAsTheUsersNotTheHarness is the
// defect this whole feature could have shipped.
//
// The wrapper claude builds around a collected prompt OPENS with
// `<task-notification>`, which is character for character the needle claude.yaml
// declares in `injected_prompts` — the needle that files a prompt under role
// `harness`. So the naive implementation of rewake makes every message the user
// types match it, and every message the user types then vanishes from their own
// side of their own conversation: a worse bug than the restart rewake removes.
//
// What separates them is the sentinel INSIDE the wrapper, which only Crowbar ever
// writes, checked before the needle is ever consulted.
func TestRegression_RewakeDeliveredPromptIsRecordedAsTheUsersNotTheHarness(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	typed := "why does the sidebar flicker on a workspace switch?"

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": rewakeWrapper(t, f, typed)})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, domain.TurnRoleUser, page.Items[0].Role,
		"a message Crowbar delivered is the user's, whatever markup the provider wrapped it in")
	assert.Equal(t, typed, page.Items[0].Text,
		"the wrapper is stripped: the record holds what the user typed, not the provider's envelope")
}

// TestRegression_RewakeDoesNotStealAGenuineHarnessInjection is the OTHER
// direction, and the one a fix aimed only at the first would break. The real
// background-subagent report carries no sentinel, so it is still the harness's —
// unchanged by everything this feature added.
func TestRegression_RewakeDoesNotStealAGenuineHarnessInjection(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, domain.TurnRoleHarness, page.Items[0].Role)
	assert.Equal(t, taskNotificationPrompt, page.Items[0].Text, "recorded verbatim, wrapper and all")
}

// TestRegression_AHarnessNotificationWearingTheWrapperShapeIsStillTheHarness
// closes the gap between the two above: the shape alone must not be enough. This
// is the document a future provider release could plausibly grow into, and it
// still has no sentinel.
func TestRegression_AHarnessNotificationWearingTheWrapperShapeIsStillTheHarness(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	lookalike := "<task-notification>\n<summary>Agent finished</summary>\n" +
		"</task-notification>\n<system-reminder>\nPONG\n</system-reminder>"

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": lookalike})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, domain.TurnRoleHarness, page.Items[0].Role)
	assert.Equal(t, lookalike, page.Items[0].Text)
}

// TestRegression_RewakeDeliveredPromptNamesTheChatAfterWhatTheUserTyped: the
// derived title is taken from the same unwrapped text, so a chat is never named
// after the provider's envelope.
func TestRegression_RewakeDeliveredPromptNamesTheChatAfterWhatTheUserTyped(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": rewakeWrapper(t, f, "rename the sidebar panel")})))
	f.wait()

	title := f.chat(t, chatID).Title
	assert.NotContains(t, title, "task-notification")
	assert.Contains(t, strings.ToLower(title), "rename")
}

// ---------------------------------------------------------------------------
// Delivery: the live session takes the message and keeps its process.
// ---------------------------------------------------------------------------

// TestSubmitPrompt_RewakeDeliversToTheLiveSessionWithoutRespawning is the point of
// the whole feature: no new process, no terminated one, and the collector holding
// exactly what the user typed.
func TestSubmitPrompt_RewakeDeliversToTheLiveSessionWithoutRespawning(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	turnEnds(t, f, runnerID, "claude", "the first turn is done")
	spawns := f.term.callCount()
	terminated := len(f.term.terminatedIDs())
	collected := f.collector(t, runnerID)
	message := "second message, same process"

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())

	require.NoError(t, err)
	assert.Equal(t, message, <-collected, "the collector carries the message verbatim")
	assert.Equal(t, spawns, f.term.callCount(), "no replacement CLI was started")
	assert.Len(t, f.term.terminatedIDs(), terminated, "the live CLI was not touched")
	assert.Equal(t, runnerID, result.RunnerID, "the delivery is attributed to the process that has it")
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID, "the chat keeps the runner it had")
}

// TestSubmitPrompt_RewakeThenItsHookConfirmsTheDelivery walks the whole round
// trip, ending where every prompt must end: the durable dispatch closed, so the
// next message is not blocked behind it.
func TestSubmitPrompt_RewakeThenItsHookConfirmsTheDelivery(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	turnEnds(t, f, runnerID, "claude", "first turn done")
	collected := f.collector(t, runnerID)
	first := "first rewake message"

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, first, uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, first, <-collected)

	// The provider wakes, wraps what the collector printed, and fires its ordinary
	// user-prompt hook — which is the acknowledgement the journal is waiting for.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": rewakeWrapper(t, f, first)})))
	turnEnds(t, f, runnerID, "claude", "answered the first")

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	var roles []string
	for _, item := range page.Items {
		if item.Text == first {
			roles = append(roles, item.Role)
		}
	}
	assert.Equal(t, []string{domain.TurnRoleUser}, roles,
		"the delivered message appears once, as the user's")

	// And the proof that the dispatch really closed: a SECOND message goes through
	// rather than meeting the pending-delivery barrier.
	second := f.collector(t, runnerID)
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "second rewake message", uuid.NewString())
	require.NoError(t, err, "a confirmed delivery must not block the next message")
	assert.Equal(t, "second rewake message", <-second)
	assert.Equal(t, 1, f.term.callCount(), "two messages, one process, from the original spawn")
}

// ---------------------------------------------------------------------------
// The fallback. The message outranks the optimisation.
// ---------------------------------------------------------------------------

// TestSubmitPrompt_NoCollectorFallsBackToTheRestartFloor is the property that
// outranks everything else here: with no channel to the live session, the message
// still arrives — carried by a replacement CLI in its argv, exactly as it was
// before rewake existed.
func TestSubmitPrompt_NoCollectorFallsBackToTheRestartFloor(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	turnEnds(t, f, runnerID, "claude", "a turn ended but nothing armed")
	require.Equal(t, 0, agentusecase.RewakeCollectors(f.usecase, runnerID),
		"the precondition: nothing is blocked on the daemon")
	spawns := f.term.callCount()
	message := "delivered the old way"

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())

	require.NoError(t, err)
	assert.Equal(t, spawns+1, f.term.callCount(), "the floor started a replacement CLI")
	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, message, call.argv[len(call.argv)-1],
		"the message the user typed is the final argv element, never dropped")
	assert.NotEqual(t, runnerID, result.RunnerID, "the replacement is a new process")
}

// TestSubmitPrompt_ACollectorThatLeavesFirstStillLosesNothing is the race the
// unbuffered handoff exists for. The collector is registered and then goes away
// before the message is offered; nothing is handed over, so the floor takes it.
func TestSubmitPrompt_ACollectorThatLeavesFirstStillLosesNothing(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	turnEnds(t, f, runnerID, "claude", "a turn ended")

	gone := make(chan struct{})
	ctx, cancel := context.WithCancel(f.ctx)
	joined := make(chan struct{})
	var once sync.Once
	agentusecase.SetRewakeJoinHook(f.usecase, func(string) { once.Do(func() { close(joined) }) })
	go func() {
		defer close(gone)
		_, _, _, _ = f.usecase.AwaitQueuedPrompt(ctx, runnerID, f.minter.Mint(runnerID), collectorBudgetMS)
	}()
	<-joined
	cancel()
	<-gone
	agentusecase.SetRewakeJoinHook(f.usecase, nil)
	spawns := f.term.callCount()
	message := "must not be lost when the collector dies"

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())

	require.NoError(t, err)
	assert.Equal(t, spawns+1, f.term.callCount())
	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, message, call.argv[len(call.argv)-1])
}

// TestSubmitPrompt_TheRestartFallbackCanStillBeConfirmed guards the seam the
// fallback introduced: the dispatch was journaled against the LIVE runner and the
// restart kills it, so without re-attribution its departure would report an
// outcome nobody can determine for a message that was provably never delivered.
func TestSubmitPrompt_TheRestartFallbackCanStillBeConfirmed(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	turnEnds(t, f, runnerID, "claude", "a turn ended")
	message := "carried by the floor"

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
	require.NoError(t, err)
	require.NotEqual(t, runnerID, result.RunnerID)

	require.NoError(t, f.usecase.IngestHook(f.ctx, result.RunnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": message})))
	turnEnds(t, f, result.RunnerID, "claude", "answered")

	// A confirmed dispatch is one the next message does not queue behind.
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "and another", uuid.NewString())
	assert.NoError(t, err, "the fallback's dispatch must close like any other")
}

// ---------------------------------------------------------------------------
// The floor is untouched for a provider that declares it.
// ---------------------------------------------------------------------------

// TestSubmitPrompt_ARestartTUIProviderNeverConsultsTheRewakeChannel: codex
// declares restart_tui, so its delivery must be exactly what it was — a
// replacement CLI with the message in argv — even with a collector blocked on its
// own runner, which is a situation only this test can construct.
func TestSubmitPrompt_ARestartTUIProviderNeverConsultsTheRewakeChannel(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	turnEnds(t, f, runnerID, "codex", "a turn ended")
	collected := f.collector(t, runnerID)
	spawns := f.term.callCount()
	message := "codex still respawns"

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())

	require.NoError(t, err)
	assert.Equal(t, spawns+1, f.term.callCount(), "restart_tui always starts a replacement")
	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, message, call.argv[len(call.argv)-1])
	assert.NotEqual(t, runnerID, result.RunnerID)
	select {
	case text, open := <-collected:
		require.False(t, open, "a restart_tui provider must never hand a message to a collector")
		_ = text
	default:
	}
	assert.Equal(t, 1, agentusecase.RewakeCollectors(f.usecase, runnerID),
		"the collector is still blocked: it was never offered anything")
}

// ---------------------------------------------------------------------------
// The collector's own contract.
// ---------------------------------------------------------------------------

func TestAwaitQueuedPrompt_RefusesAWrongCredential(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")
	otherID := uuid.NewString()

	_, _, _, err := f.usecase.AwaitQueuedPrompt(f.ctx, runnerID, f.minter.Mint(otherID), 1000)

	require.ErrorIs(t, err, agenttools.ErrUnauthorized,
		"a segment id is published on the chats API; the token is the credential")
}

// TestAwaitQueuedPrompt_EndsForARunnerThatIsGone is what reaps the collector a
// provider leaves behind. It is spawned DETACHED, so the CLI dying does not end
// it, and a poll that answered "nothing yet" forever would be an orphan holding a
// socket for the life of the daemon.
func TestAwaitQueuedPrompt_EndsForARunnerThatIsGone(t *testing.T) {
	f := newFixture(t)
	unknown := uuid.NewString()

	_, ok, _, err := f.usecase.AwaitQueuedPrompt(f.ctx, unknown, f.minter.Mint(unknown), 1000)

	require.Error(t, err, "an error is what stops the collector asking again")
	assert.False(t, ok)
}

// TestAwaitQueuedPrompt_ReturnsEmptyHandedWhenTheCallerLeaves pins the ordinary
// outcome: no message, no error, and the collector is expected to ask again.
func TestAwaitQueuedPrompt_ReturnsEmptyHandedWhenTheCallerLeaves(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")
	ctx, cancel := context.WithCancel(f.ctx)
	joined := make(chan struct{})
	var once sync.Once
	agentusecase.SetRewakeJoinHook(f.usecase, func(string) { once.Do(func() { close(joined) }) })
	t.Cleanup(func() { agentusecase.SetRewakeJoinHook(f.usecase, nil) })

	type outcome struct {
		ok  bool
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		_, ok, _, err := f.usecase.AwaitQueuedPrompt(ctx, runnerID, f.minter.Mint(runnerID), collectorBudgetMS)
		done <- outcome{ok: ok, err: err}
	}()
	<-joined
	cancel()

	got := <-done
	assert.False(t, got.ok)
	assert.NoError(t, got.err, "an abandoned poll is not a fault")
	assert.Equal(t, 0, agentusecase.RewakeCollectors(f.usecase, runnerID),
		"a collector that leaves must free its slot, or a message could be handed to nobody")
}

// TestAwaitQueuedPrompt_TwoCollectorsShareOneMessage: a provider can arm more than
// one collector, and a message must reach exactly one of them. The delivery is an
// unbuffered handoff, so this is a property of the transport rather than of a
// lock somebody remembered to take.
func TestAwaitQueuedPrompt_TwoCollectorsShareOneMessage(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	turnEnds(t, f, runnerID, "claude", "a turn ended")
	first := f.collector(t, runnerID)
	second := f.collector(t, runnerID)
	require.Equal(t, 2, agentusecase.RewakeCollectors(f.usecase, runnerID))

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "exactly once", uuid.NewString())
	require.NoError(t, err)

	got := 0
	select {
	case text := <-first:
		if text != "" {
			got++
		}
	case text := <-second:
		if text != "" {
			got++
		}
	case <-time.After(5 * time.Second):
	}
	assert.Equal(t, 1, got, "one message, one collector")
	assert.Equal(t, 1, agentusecase.RewakeCollectors(f.usecase, runnerID),
		"the other collector is still blocked, waiting for a message of its own")
}
