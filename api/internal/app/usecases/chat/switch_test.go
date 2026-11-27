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

func TestRegression_SwitchInsideTheUnacknowledgedPromptWindowWaitsForTheAnswer(t *testing.T) {
	f, chatID, carrier, done := switchParkedOnAPendingPrompt(t)

	prompt(t, f, carrier, "claude", "reply with only: SEED")

	got := awaitSwitch(t, done)
	require.ErrorIs(t, got.err, agentusecase.ErrPromptBusy, "the acknowledged prompt's turn is running")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, carrier, live.ID)

	turn(t, f, carrier, "claude", "SEED")
	switched, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	assertSwitchedToCodex(t, f, chatID, switched)
	incoming := f.term.calls[f.term.callCount()-1]
	assert.Contains(t, strings.Join(incoming.argv, "\x00"), "SEED",
		"the handoff carries the turn that ran")
}

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

func TestRegression_StopCancelsASwitchParkedOnAPendingPrompt(t *testing.T) {
	f, chatID, _, done := switchParkedOnAPendingPrompt(t)
	spawns := f.term.callCount()

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))

	got := awaitSwitch(t, done)
	require.ErrorIs(t, got.err, agentusecase.ErrStopped)
	assert.Empty(t, got.runnerID)
	assert.Equal(t, spawns, f.term.callCount(), "the preempted switch spawned nothing")
}

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
		id, switchErr := f.usecase.SwitchProvider(context.Background(), chatID, "codex", "")
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

func TestSwitchProvider_ResolvesCwdThroughTheAncestorWalkForABubble(t *testing.T) {
	f := newFixture(t)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")
	f.ws.worktreeDirIDs = nil

	_, err := f.usecase.SwitchProvider(f.ctx, bubbleID, "codex", "")
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

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	f.wait()

	assert.Contains(t, f.term.terminatedIDs(), oldTerm, "the outgoing CLI is quit gracefully")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID)
	assert.Equal(t, "codex", live.ProviderID)
	assert.NotEqual(t, oldTerm, live.TerminalSession, "the incoming CLI has its own PTY")

	assert.False(t, f.isLive(oldRunner))
	exit, ok := f.lastExit(t, chatID)
	require.True(t, ok)
	assert.Equal(t, ledger.RunnerExit{RunnerID: oldRunner, Reason: domain.AgentExitDisplaced, At: exit.At}, exit)
}

func TestSwitchProvider_TerminateFailure_SessionAlreadyGone_ContinuesSwitch(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.term.terminateErr = fmt.Errorf("terminal: terminate: %w: term-1", engineterminal.ErrSessionNotFound)

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	require.NotEmpty(t, newRunner)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID)
}

func TestSwitchProvider_TerminateFailure_OtherError_AbortsSwitch(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminate outgoing terminal")

	require.Equal(t, 1, f.term.callCount(), "no second CLI may be spawned after a real terminate failure")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, oldRunner, live.ID, "the chat still belongs to the CLI that would not die")
}

func TestSwitchProvider_AssembleHandoffFailure_AbortsBeforeTerminate(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")

	f.ws.err = errors.New("boom: worktree lookup")
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.Error(t, err)

	f.ws.err = nil
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

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	require.NotEmpty(t, newRunner)

	require.Equal(t, 2, f.term.callCount())
	newCall := f.term.calls[1]
	assert.Equal(t, "codex", filepath.Base(newCall.argv[0]))
	assert.Contains(t, strings.Join(newCall.argv, "\x00"), "prior turn content for handoff")
}

func TestSwitchProvider_Broadcasts_TheChatsOwnProviderChange(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.bc.reset()
	f.snapFrames.reset()

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	f.wait()

	assert.Equal(t, []string{"provider_set"}, f.bcKinds(t),
		"the only chat-aggregate write a switch makes is the chat's own vendor moving")
	assert.Equal(t, []string{"exited", "started"}, f.runnerKinds(t))
}

func TestSwitchProvider_LaunchesTheIncomingProviderOnTheUsersSurface(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	require.Empty(t, f.chat(t, chatID).Surface)

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", domain.SurfaceTerminal)
	require.NoError(t, err)
	f.wait()

	assert.Equal(t, domain.SurfaceTerminal, f.chat(t, chatID).Surface)
}

func TestSwitchProvider_RecordsAProviderSwitchedInterruption(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	f.wait()

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, ledger.MarkerProviderSwitched, ints[0].Kind)
	assert.Equal(t, "codex", ints[0].Detail)
	assert.NotNil(t, ints[0].ResolvedAt, "Crowbar's own doing: opened and resolved together")
}

func TestSwitchProvider_SwitchingToTheSameProvider_RecordsNothing(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude", "")
	require.NoError(t, err)
	f.wait()

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, ints, "switching to the provider already on the chat is not a switch")
}

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

	claudeRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude", "")
	require.NoError(t, err)
	f.wait()

	turn(t, f, claudeRunner, "claude", "claude spoke while codex was away")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "codex", live.ProviderID)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv
	assert.Equal(t, "sid-codex-native", argAfter(t, argv, "resume"),
		"the resume subcommand must name codex's own prior thread: %v", argv)
	msg := argv[len(argv)-1]
	assert.Contains(t, msg, "claude spoke while codex was away", "the gap rides the resume: %v", argv)
	assert.NotContains(t, msg, "codex ledger content", "codex is not re-fed its own turns")
	assert.NotContains(t, msg, "[Crowbar]", "the real gap, not a pointer: %v", argv)
}

func TestSwitchProvider_ForwardSwitch_CarriesWholeConversation(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	turn(t, f, runnerID, "claude", "claude said this first")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)

	require.Equal(t, 2, f.term.callCount())
	argv := f.term.calls[1].argv

	assert.Equal(t, -1, slices.Index(argv, "resume"), "a provider new to the chat has no conversation to resume")

	doc := configValue(t, argv, "developer_instructions=")
	assert.Contains(t, doc, "claude said this first")
}

func TestSwitchProvider_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.SwitchProvider(f.ctx, "does-not-exist", "codex", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch provider: chat")
}

func TestSwitchProvider_DormantChat_SwitchesAnyway(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "precondition: the chat is dormant")

	before := len(f.term.terminateRequestIDs())
	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
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

	f.ws.err = errors.New("boom: workspace lookup")
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preflight worktree dir")
	assert.ErrorContains(t, err, "boom: workspace lookup")
}

func TestSwitchProvider_ThePickFollowsTheProvider(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	ranAs(t, f, chatID, "sonnet")

	assert.NotContains(t, switchedTo(t, f, chatID, "codex"), "--model")
	assert.Empty(t, f.chat(t, chatID).Model, "codex never ran here: its default")
	assert.Equal(t, []string{"provider_switched:codex", "model_changed:"}, f.markers(t, chatID))
	ranAs(t, f, chatID, "gpt-5")

	assert.Equal(t, "sonnet", argAfter(t, switchedTo(t, f, chatID, "claude"), "--model"))
	assert.Equal(t, "sonnet", f.chat(t, chatID).Model)

	assert.Equal(t, "gpt-5", argAfter(t, switchedTo(t, f, chatID, "codex"), "--model"))
	assert.Equal(t, "gpt-5", f.chat(t, chatID).Model)
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "",
		&domain.ChatSelection{Model: "gpt-5"})
	require.NoError(t, err, "the composer re-posts the held pick")
}

func ranAs(t *testing.T, f testFixture, chatID, model string) {
	t.Helper()
	text := "run as " + model
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, text, uuid.NewString(), "", &domain.ChatSelection{Model: model})
	require.NoError(t, err)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	require.Equal(t, model, live.LaunchModel)
	prompt(t, f, live.ID, live.ProviderID, text)
	turn(t, f, live.ID, live.ProviderID, "ran as "+model)
}

func switchedTo(t *testing.T, f testFixture, chatID, provider string) []string {
	t.Helper()
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, provider, "")
	require.NoError(t, err)
	return f.term.calls[f.term.callCount()-1].argv
}
