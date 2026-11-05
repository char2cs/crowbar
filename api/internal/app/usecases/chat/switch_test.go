package chat_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// TestRegression_SwitchInsideTheUnacknowledgedPromptWindowWaitsForTheAnswer is
// the 409 reproduced twice on 2026-09-23: a switch sent between SubmitPrompt's 200
// and the provider's user_prompt hook, while the chat still read idle. The switch
// waits for the ledger to decide the prompt: acknowledged, it opened a turn, and a
// switch while a turn runs is refused.
func TestRegression_SwitchInsideTheUnacknowledgedPromptWindowWaitsForTheAnswer(t *testing.T) {
	f, chatID, carrier, done := switchParkedOnAPendingPrompt(t)

	prompt(t, f, carrier, "claude", "reply with only: SEED")

	got := awaitSwitch(t, done)
	require.ErrorIs(t, got.err, agentusecase.ErrPromptBusy, "the acknowledged prompt's turn is running")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, carrier, live.ID)

	turn(t, f, carrier, "claude", "SEED")
	switched, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	assertSwitchedToCodex(t, f, chatID, switched)
	incoming := f.term.calls[f.term.callCount()-1]
	assert.Contains(t, strings.Join(incoming.argv, "\x00"), "SEED",
		"the handoff carries the turn that ran")
}

// A prompt the provider goes idle without taking is settled, and the settle
// releases the parked switch exactly as an acknowledgement would.
func TestRegression_ASwitchParkedOnAPromptGoesAheadOnceThePromptSettles(t *testing.T) {
	f, chatID, carrier, done := switchParkedOnAPendingPrompt(t)

	require.NoError(t, f.usecase.IngestHook(f.ctx, carrier, "claude", "idle", mustJSON(t,
		map[string]any{"message": "Claude is waiting for your input"})))

	got := awaitSwitch(t, done)
	require.NoError(t, got.err)
	assertSwitchedToCodex(t, f, chatID, got.runnerID)
	prompts := crowbarPrompts(t, f, chatID)
	require.Len(t, prompts, 1)
	assert.Equal(t, ledger.ReasonNotAcknowledged, prompts[0].Reason)
}

// Stop, which cancels the chat's parked waits, is the one thing besides the ledger that
// ends the wait, and it starts and kills nothing on the switch's behalf.
func TestRegression_StopCancelsASwitchParkedOnAPendingPrompt(t *testing.T) {
	f, chatID, _, done := switchParkedOnAPendingPrompt(t)
	spawns := f.term.callCount()

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))

	got := awaitSwitch(t, done)
	require.ErrorIs(t, got.err, agentusecase.ErrStopped)
	assert.Empty(t, got.runnerID)
	assert.Equal(t, spawns, f.term.callCount(), "the preempted switch spawned nothing")
}

// switchParkedOnAPendingPrompt submits a prompt and starts a switch to codex,
// returning once the switch has parked on that unacknowledged prompt.
func switchParkedOnAPendingPrompt(t *testing.T) (testFixture, string, string, <-chan switchResult) {
	t.Helper()
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "reply with only: SEED", uuid.NewString(), "", nil)
	require.NoError(t, err)
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"precondition: the prompt is recorded and unacknowledged")
	require.False(t, f.busy(t, chatID), "precondition: Working is set by the hook that has not arrived")
	carrier, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)

	parked := parkedOn(t, agentusecase.WaitingForPendingPromptLog)
	done := make(chan switchResult, 1)
	go func() {
		id, switchErr := f.usecase.SwitchProvider(context.Background(), chatID, "codex")
		done <- switchResult{runnerID: id, err: switchErr}
	}()
	select {
	case <-parked:
	case got := <-done:
		t.Fatalf("the switch answered %v inside the unacknowledged-prompt window instead of waiting", got.err)
	}
	return f, chatID, carrier.ID, done
}

func awaitSwitch(t *testing.T, done <-chan switchResult) switchResult {
	t.Helper()
	deadline, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	select {
	case got := <-done:
		return got
	case <-deadline.Done():
		t.Fatal("the parked switch never finished")
		return switchResult{}
	}
}

func assertSwitchedToCodex(t *testing.T, f testFixture, chatID, switched string) {
	t.Helper()
	f.wait()
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, switched, live.ID)
	assert.Equal(t, "codex", live.ProviderID)
}

// parkedOn is closed when the usecase logs message, which it does immediately
// before it blocks: a test asserts at a moment it knows the switch has reached.
func parkedOn(t *testing.T, message string) <-chan struct{} {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	h := &parkHandler{message: message, ch: make(chan struct{})}
	slog.SetDefault(slog.New(h))
	return h.ch
}

type parkHandler struct {
	message string
	once    sync.Once
	ch      chan struct{}
}

func (h *parkHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *parkHandler) Handle(_ context.Context, r slog.Record) error {
	if strings.Contains(r.Message, h.message) {
		h.once.Do(func() { close(h.ch) })
	}
	return nil
}

func (h *parkHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *parkHandler) WithGroup(string) slog.Handler { return h }

// A switch's preflight resolves a bubble's cwd through the ancestor walk too.
// The whole call list is asserted: the spawn resolves "ws1" last either way, and
// fakeWorkspace answers "" as readily as a real id.
func TestSwitchProvider_ResolvesCwdThroughTheAncestorWalkForABubble(t *testing.T) {
	f := newFixture(t)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")
	f.ws.worktreeDirIDs = nil

	_, err := f.usecase.SwitchProvider(f.ctx, bubbleID, "codex")
	require.NoError(t, err)
	f.wait()

	require.NotEmpty(t, f.ws.worktreeDirIDs)
	assert.NotContains(t, f.ws.worktreeDirIDs, "",
		"every cwd a bubble's switch resolves must come from its workspace-owning ancestor")
}

func TestSwitchProvider_TerminatesOutgoingCLI_AndTakesOverTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")
	oldTerm := f.runner(t, oldRunner).TerminalSession
	require.NotEmpty(t, oldTerm)

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	assert.Contains(t, f.term.terminatedIDs(), oldTerm, "the outgoing CLI is quit gracefully")

	// The chat did not move and was not written to; only the runner on it changed.
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID)
	assert.Equal(t, "codex", live.ProviderID)
	assert.NotEqual(t, oldTerm, live.TerminalSession, "the incoming CLI has its own PTY")

	// The switch returned only once the outgoing CLI's exit was recorded: a
	// replacement never lands beside it.
	assert.False(t, f.isLive(oldRunner))
	exit, ok := f.lastExit(t, chatID)
	require.True(t, ok)
	assert.Equal(t, ledger.RunnerExit{RunnerID: oldRunner, Reason: domain.AgentExitDisplaced, At: exit.At}, exit)
}

// TestSwitchProvider_TerminateFailure_SessionAlreadyGone_ContinuesSwitch: when
// TerminateGraceful fails because the terminal session is already gone (the one error
// the real engine returns today), the switch must still proceed.
func TestSwitchProvider_TerminateFailure_SessionAlreadyGone_ContinuesSwitch(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.term.terminateErr = fmt.Errorf("terminal: terminate: %w: term-1", engineterminal.ErrSessionNotFound)

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newRunner)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID)
}

// TestSwitchProvider_TerminateFailure_OtherError_AbortsSwitch: a TerminateGraceful
// failure that is NOT "session already gone" must abort the switch entirely rather
// than leave two live CLIs pointed at one chat.
func TestSwitchProvider_TerminateFailure_OtherError_AbortsSwitch(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminate outgoing terminal")

	require.Equal(t, 1, f.term.callCount(), "no second CLI may be spawned after a real terminate failure")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, oldRunner, live.ID, "the chat still belongs to the CLI that would not die")
}

// TestSwitchProvider_AssembleHandoffFailure_AbortsBeforeTerminate: the handoff is
// assembled BEFORE the terminate, so a failure there leaves the chat completely
// untouched — rather than killing the old CLI and spawning the new one with an EMPTY
// handoff.
func TestSwitchProvider_AssembleHandoffFailure_AbortsBeforeTerminate(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")

	f.ws.err = errors.New("boom: worktree lookup")
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.Error(t, err)

	f.ws.err = nil // let the assertion reads resolve the worktree again
	assert.Empty(t, f.term.terminatedIDs(), "the outgoing CLI must never be terminated when the handoff fails first")
	require.Equal(t, 1, f.term.callCount(), "and no new CLI is spawned")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, oldRunner, live.ID)
}

func TestSwitchProvider_Forward_SpawnsTargetProviderWithHandoff(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	turn(t, f, runnerID, "claude", "prior turn content for handoff")

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newRunner)

	require.Equal(t, 2, f.term.callCount())
	newCall := f.term.calls[1]
	// Basename: argv[0] is binpath.Resolve'd to the CLI's absolute path when installed.
	assert.Equal(t, "codex", filepath.Base(newCall.argv[0]))
	assert.Contains(t, strings.Join(newCall.argv, "\x00"), "prior turn content for handoff")
}

// TestSwitchProvider_Broadcasts_TheChatsOwnProviderChange: a handoff changes which
// CLI is on the chat, and the runner feed carries that whole story in order — the
// outgoing runner exits, and only then does the incoming one start.
//
// The chat aggregate is written to exactly ONCE, and only for what durably moved: its
// own vendor (domain.Chat.ProviderID), the same way SwitchToTerminal writes its
// surface. A respawn on the SAME provider changes nothing and emits nothing — the
// command refuses an unchanged value — so this frame appears only on a real switch.
func TestSwitchProvider_Broadcasts_TheChatsOwnProviderChange(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.bc.reset()
	f.snapFrames.reset()

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	assert.Equal(t, []string{"provider_set"}, f.bcKinds(t),
		"the only chat-aggregate write a switch makes is the chat's own vendor moving")
	assert.Equal(t, []string{"exited", "started"}, f.runnerKinds(t))
}

// TestSwitchProvider_RecordsAProviderSwitchedInterruption guards the new
// InterruptProviderSwitched marker: recorded once the switch actually
// commits, and only when the target differs from the chat's provider
// beforehand — Crowbar's own doing, not something a provider hook reports.
func TestSwitchProvider_RecordsAProviderSwitchedInterruption(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, ledger.MarkerProviderSwitched, ints[0].Kind)
	assert.Equal(t, "codex", ints[0].Detail)
	assert.NotNil(t, ints[0].ResolvedAt, "Crowbar's own doing: opened and resolved together")
}

// TestSwitchProvider_SwitchingToTheSameProvider_RecordsNothing guards the
// no-op case ResumeChat exercises every day: resuming into the provider the
// chat is ALREADY on is not a switch worth marking.
func TestSwitchProvider_SwitchingToTheSameProvider_RecordsNothing(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, ints, "switching to the provider already on the chat is not a switch")
}

// With no api connection (this fixture disables it, as production degrades to
// when app-server fails), the hooks PTY is the only codex: it must carry the
// native `resume {id}` itself, or codex mints a new thread on every switch back.
// Its sessions live in its own home, which Crowbar must never own and so delete.
func TestSwitchProvider_SwitchBack_ResumesItsOwnSessionOverTheOnlyCodexThereIs(t *testing.T) {
	f := newFixture(t)

	chatID, codexRunner := f.spawn(t, "codex")
	f.announce(t, codexRunner, "sid-codex-native")
	exchange(t, f, codexRunner, "codex", "codex ledger content")
	for _, call := range f.term.calls {
		for _, kv := range call.env {
			assert.False(t, strings.HasPrefix(kv, "CODEX_HOME=") && kv != "CODEX_HOME="+os.Getenv("CODEX_HOME"),
				"Crowbar must never own codex's home — its sessions live there")
		}
	}

	claudeRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	// What codex misses while it is away.
	turn(t, f, claudeRunner, "claude", "claude spoke while codex was away")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "codex", live.ProviderID)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv
	assert.Equal(t, "sid-codex-native", argAfter(t, argv, "resume"),
		"the resume subcommand must name codex's own prior thread: %v", argv)
	for _, tok := range argv {
		assert.NotContains(t, tok, "[Crowbar]",
			"codex declares no resume_context_inject: its gap never rides argv as a pointer it would answer "+
				"as an unrelated second conversation: %v", argv)
	}
}

// TestSwitchProvider_ForwardSwitch_CarriesWholeConversation is the other half of the
// gap rule: a provider that has never run in this chat has no conversation to resume
// and therefore no history at all, so it gets the ENTIRE ledger.
func TestSwitchProvider_ForwardSwitch_CarriesWholeConversation(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	turn(t, f, runnerID, "claude", "claude said this first")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)

	require.Equal(t, 2, f.term.callCount())
	argv := f.term.calls[1].argv

	// codex is new to this chat: no resume, and the context rides the silent
	// developer_instructions channel — never the positional user prompt.
	assert.Equal(t, -1, slices.Index(argv, "resume"), "a provider new to the chat has no conversation to resume")

	doc := configValue(t, argv, "developer_instructions=")
	assert.Contains(t, doc, "claude said this first")
}

func TestSwitchProvider_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.SwitchProvider(f.ctx, "does-not-exist", "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch provider: chat")
}

// TestSwitchProvider_DormantChat_SwitchesAnyway: a chat whose CLI is gone (it exited,
// or died with the daemon) used to be a hard dead end — the pane told the user to
// "switch provider below to start a new one" while this call returned ErrNotFound, so
// the chat could never be re-entered by ANY route. A dormant chat now simply means
// there is no outgoing CLI to quit.
func TestSwitchProvider_DormantChat_SwitchesAnyway(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "precondition: the chat is dormant")

	before := len(f.term.terminateRequestIDs())
	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newRunner)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "codex", live.ProviderID)
	assert.Len(t, f.term.terminateRequestIDs(), before, "a dormant chat has no CLI to terminate")
}

func TestSwitchProvider_WorkspaceReaderFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	// A workspace-reader failure surfaces wrapped, at the preflight WorktreeDir
	// read: the wait for a pending prompt reads only the chat ledger before it.
	f.ws.err = errors.New("boom: workspace lookup")
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preflight worktree dir")
	assert.ErrorContains(t, err, "boom: workspace lookup")
}

// The pane's provider switch keeps the outgoing provider's model as the chat's
// pick, and the composer re-posts that pick with its next send: a pick the chat
// already holds changes nothing, so it must not be refused against the new provider.
func TestRegression_ASendAfterASwitchRepostingTheHeldPickIsDelivered(t *testing.T) {
	f := newFixture(t)
	// codex's catalogue is discovered, so any pick holds there; claude's is declared.
	chatID, _ := f.spawn(t, "codex")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "gpt-5", ""))
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "",
		&domain.ChatSelection{Model: "gpt-5"})
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)
	assert.Empty(t, live.LaunchModel, "the stale pick resolves to claude's default")
}
