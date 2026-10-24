package ingress

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func descriptorFor(t *testing.T, provider string) engineagents.Agent {
	t.Helper()
	agent, err := engineagents.New().Get(t.Context(), t.TempDir(), provider)
	require.NoError(t, err)
	return agent
}

// showingRunners answers only ShowingNativeView: surfaceGated is all that is
// under test here.
type showingRunners struct {
	RecorderRunners
	showing bool
}

func (r showingRunners) ShowingNativeView(string) bool { return r.showing }

// An event that declares no surfaces is on every one, without ever asking
// which surface is showing.
func TestSurfaceGated_AnEventWithNoDeclaredSurfacesIsNeverGated(t *testing.T) {
	t.Parallel()
	r := &Recorder{runners: showingRunners{showing: true}}

	assert.False(t, r.surfaceGated("runner-1", descriptorFor(t, "codex"), "turn_stop"))
}

// A chat-only event is not listened to while the provider's own view shows.
func TestSurfaceGated_AChatOnlyEventIsGatedOffWhileTheNativeViewIsShowing(t *testing.T) {
	t.Parallel()
	r := &Recorder{runners: showingRunners{showing: true}}

	assert.True(t, r.surfaceGated("runner-1", descriptorFor(t, "codex"), "message_delta"))
}

func TestSurfaceGated_AChatOnlyEventFlowsWhenTheNativeViewIsNotShowing(t *testing.T) {
	t.Parallel()
	r := &Recorder{runners: showingRunners{showing: false}}

	assert.False(t, r.surfaceGated("runner-1", descriptorFor(t, "codex"), "message_delta"))
}

type fakeChats struct {
	agentchat.EventStore
	err error
}

func (f fakeChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	if f.err != nil {
		return domain.Chat{}, f.err
	}
	if id != testChat {
		return domain.Chat{}, agentchat.ErrNotFound
	}
	return domain.Chat{ID: testChat, WorkspaceID: "ws-1"}, nil
}

type fakeWorkspace struct {
	seam.WorkspaceReader
	home string
	err  error
}

func (w fakeWorkspace) WorktreeDir(context.Context, string) (string, string, string, string, error) {
	return w.home, "", "", "", w.err
}

func (w fakeWorkspace) AgentChatsDir(context.Context, string) (string, error) {
	return filepath.Join(w.home, "chats"), w.err
}

type recordedTitles struct {
	mu     sync.Mutex
	titles []string
}

func (r *recordedTitles) RenameChat(_ context.Context, _, title, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.titles = append(r.titles, title)
	return errors.New("titles are not kept here")
}

type recorderRunners struct {
	ending  bool
	started []engineagents.CanonicalEvent
}

func (r *recorderRunners) TurnRunner(_ string, open func(engineagents.Runner) error) (bool, error) {
	if r.ending {
		return false, nil
	}
	return true, open(engineagents.Runner{})
}

func (r *recorderRunners) HandleSessionStart(_ context.Context, _ engineagents.Runner, ev engineagents.CanonicalEvent) error {
	r.started = append(r.started, ev)
	return nil
}

func (r *recorderRunners) TelemetryOnChatSurface(context.Context, string) bool { return true }

func (r *recorderRunners) ShowingNativeView(string) bool { return false }

type frame struct {
	messageID, text, kind string
	startedAt             time.Time
}

type feedSpy struct {
	mu        sync.Mutex
	frames    []frame
	telemetry int
}

func (f *feedSpy) feed() seam.ChatFeed {
	return seam.ChatFeed{
		MessageDelta: func(_, _, messageID, text, kind string, startedAt time.Time) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.frames = append(f.frames, frame{messageID, text, kind, startedAt})
		},
		Telemetry: func(string, string, engineagents.Telemetry) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.telemetry++
		},
	}
}

// recorderFixture is a Recorder over a real ledger and the shipped descriptor
// of provider, recording for runner run-1, the chat's live runner.
type recorderFixture struct {
	r       *Recorder
	ledger  *ledgerstore.Store
	agents  engineagents.Agents
	agent   engineagents.Agent
	runner  engineagents.Runner
	runners *recorderRunners
	titles  *recordedTitles
	answers *answerdesk.Desk
	feed    *feedSpy
	blobs   *ledgerstore.Blobs
	blobDir string
}

func newRecorderFixture(t *testing.T, provider string) *recorderFixture {
	t.Helper()
	home := t.TempDir()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	store, err := ledgerstore.Open(db, time.Now)
	require.NoError(t, err)
	blobDir := t.TempDir()
	blobs, err := ledgerstore.OpenBlobs(blobDir)
	require.NoError(t, err)
	agents := engineagents.New()
	agent, err := agents.Get(t.Context(), home, provider)
	require.NoError(t, err)
	f := &recorderFixture{
		ledger: store, agents: agents, agent: agent, runners: &recorderRunners{}, titles: &recordedTitles{},
		answers: answerdesk.New(time.Minute, nil), feed: &feedSpy{}, blobs: blobs, blobDir: blobDir,
		runner: engineagents.Runner{ID: "run-1", ProviderID: provider, WorkspaceID: "ws-1", CurrentChatID: testChat},
	}
	f.r = NewRecorder(RecorderDeps{
		Chats: fakeChats{}, Agents: agents, Workspace: fakeWorkspace{home: home},
		Home: func() (string, error) { return home, nil }, Ledger: store, Blobs: blobs,
		Telemetry: telemetry.New(), Answers: f.answers, Titles: f.titles,
	})
	f.r.SetRunners(f.runners)
	f.r.SetFeed(f.feed.feed())
	_, _, err = store.Append(t.Context(), testChat,
		row(t, ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{RunnerID: "run-1", Provider: provider}))
	require.NoError(t, err)
	return f
}

func (f *recorderFixture) dispatch(t *testing.T, ev engineagents.CanonicalEvent) ([]byte, string, error) {
	t.Helper()
	return f.r.Dispatch(t.Context(), f.runner, f.agent, ev, []byte(`{}`), engineagents.ChannelHooks)
}

func (f *recorderFixture) rows(t *testing.T) []ledger.Row {
	t.Helper()
	rows, err := f.ledger.Read(t.Context(), testChat, 0, 0)
	require.NoError(t, err)
	return rows
}

func (f *recorderFixture) state(t *testing.T) ledger.State {
	t.Helper()
	st, err := f.ledger.State(t.Context(), testChat)
	require.NoError(t, err)
	return st
}

// said is what rows say the agent said, message by message.
func said(rows []ledger.Row) []string {
	var fold ledger.MessageFold
	for _, r := range rows {
		fold.Apply(r)
	}
	var out []string
	for _, m := range fold.Messages() {
		out = append(out, m.Text)
	}
	return out
}

func TestRecorder_APromptOpensATurnAndTitlesTheChat(t *testing.T) {
	f := newRecorderFixture(t, "claude")

	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "\n  fix the build\nplease"})

	require.NoError(t, err)
	turn, running := f.state(t).RunningTurn()
	require.True(t, running)
	assert.Equal(t, "run-1", turn.RunnerID)
	assert.Equal(t, []string{"fix the build"}, f.titles.titles, "a title the chat's words derive, best-effort")
}

// Crowbar's own injected context is nobody's words: its echo opens the turn and
// says nothing.
func TestRecorder_AnInjectedEchoSaysNothing(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	f.agents.RecordInjection("run-1", "handoff blob")

	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "handoff blob"})

	require.NoError(t, err)
	assert.Equal(t, []ledger.Kind{ledger.KindRunnerStarted, ledger.KindTurnStarted}, kinds(f.rows(t)))
	assert.Empty(t, f.titles.titles)
}

func TestRecorder_AHookOfAChatGoneIsRefused(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	ev := engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "x"}

	nowhere := f.runner
	nowhere.CurrentChatID = ""
	_, _, err := f.r.Dispatch(t.Context(), nowhere, f.agent, ev, nil, engineagents.ChannelHooks)
	require.ErrorIs(t, err, apperr.ErrNotFound)

	deleted := f.runner
	deleted.CurrentChatID = "chat-deleted"
	_, _, err = f.r.Dispatch(t.Context(), deleted, f.agent, ev, nil, engineagents.ChannelHooks)
	require.ErrorIs(t, err, apperr.ErrNotFound)

	f.r.chats = fakeChats{err: errors.New("disk")}
	_, _, err = f.dispatch(t, ev)
	require.ErrorContains(t, err, "disk")
}

func TestRecorder_SessionEventsAreTheRunnersNotTheLedgers(t *testing.T) {
	f := newRecorderFixture(t, "claude")

	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookSessionStart, SessionID: "s-1"})
	require.NoError(t, err)
	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookSessionEnd})
	require.NoError(t, err)

	require.Len(t, f.runners.started, 1)
	assert.Len(t, f.rows(t), 1)

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookTelemetry})
	require.ErrorIs(t, err, seam.ErrHookNotRecorded)
}

// An event of a session the chat is not bound to is foreign, unless it is a
// nested conversation the chat's agent opened.
func TestRecorder_AnotherSessionsEventIsForeignUnlessNested(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	bound, err := ledger.Announce(f.state(t), "claude", "main", false)
	require.NoError(t, err)
	_, _, err = f.ledger.Append(t.Context(), testChat, bound...)
	require.NoError(t, err)

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPre, SessionID: "stranger",
		Tool: &engineagents.ToolEvent{ID: "t1", Name: "Bash"},
	})
	require.ErrorIs(t, err, engineagents.ErrForeignConversation)

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, SessionID: "main", Message: "go"})
	require.NoError(t, err)
	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPost, SessionID: "main",
		Tool: &engineagents.ToolEvent{ID: "spawn", Name: "Agent", NestedSessionID: "child"},
	})
	require.NoError(t, err)
	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPre, SessionID: "child",
		Tool: &engineagents.ToolEvent{ID: "t1", Name: "Bash"},
	})
	require.NoError(t, err)

	rows := f.rows(t)
	assert.Equal(t, "child", rows[len(rows)-1].Scope)
}

func TestRecorder_AHeldAskIsParkedOnTheDesk(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)

	_, ask, err := f.dispatch(t, engineagents.CanonicalEvent{
		Kind:   engineagents.HookPermission,
		Choice: &engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"},
	})

	require.NoError(t, err)
	require.NotEmpty(t, ask)
	_, held := f.answers.Pending(ask)
	assert.True(t, held)
}

func TestRecorder_AToolsPayloadsAreStoredBeforeItsRows(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPost,
		Tool: &engineagents.ToolEvent{ID: "t1", Name: "Bash", Input: []byte(`{"command":"ls"}`), Result: []byte("a.go")},
	})

	require.NoError(t, err)
	rows := f.rows(t)
	call := bodyOf[ledger.ToolCall](t, rows[len(rows)-1])
	assert.NotEmpty(t, call.InputRef)
	assert.NotEmpty(t, call.ResultRef)
	refs, err := f.ledger.BlobRefs(t.Context(), testChat)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{call.InputRef, call.ResultRef}, refs)
}

func TestRecorder_AFailedToolsWholeErrorIsItsResult(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	whole := strings.Repeat("é", 40_000)

	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolFail, Tool: &engineagents.ToolEvent{ID: "t1", Name: "Bash", Error: whole},
	})

	require.NoError(t, err)
	rows := f.rows(t)
	call := bodyOf[ledger.ToolCall](t, rows[len(rows)-1])
	assert.Less(t, len(call.Error), len(whole), "its row keeps a caption")
	got, err := f.blobs.Get(call.ResultRef)
	require.NoError(t, err)
	assert.Equal(t, whole, string(got))
}

func TestRecorder_AToolIsRecordedWithoutThePayloadsItCannotStore(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	f := newRecorderFixture(t, "claude")
	require.NoError(t, os.Chmod(f.blobDir, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(f.blobDir, 0o700)) })

	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPost,
		Tool: &engineagents.ToolEvent{ID: "t1", Name: "Bash", Input: []byte("args"), Result: []byte("out")},
	})

	require.NoError(t, err)
	rows := f.rows(t)
	call := bodyOf[ledger.ToolCall](t, rows[len(rows)-1])
	assert.Equal(t, "Bash", call.Name)
	assert.Empty(t, call.InputRef)
	assert.Empty(t, call.ResultRef)
}

// A turn's end on a runner whose own end is under way is recorded unheld: it
// hands no steer over.
func TestRecorder_AnEndOnAnEndingRunnerIsRecordedUnheld(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)
	f.runners.ending = true

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: "done"})

	require.NoError(t, err)
	_, running := f.state(t).RunningTurn()
	assert.False(t, running)
}

// Every frame carries the stream so far; what its turn's end recorded is
// dropped with it.
func TestRecorder_StreamedTextIsShownWholeUntilItsTurnEnds(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)

	for _, ev := range []engineagents.CanonicalEvent{
		delta(engineagents.HookMessageDelta, "m1", false, false, "the "),
		delta(engineagents.HookMessageDelta, "m1", false, false, "answer"),
		delta(engineagents.HookReasoningDelta, "r1", false, false, "hm"),
	} {
		_, _, err = f.dispatch(t, ev)
		require.NoError(t, err)
	}
	require.Len(t, f.feed.frames, 3)
	assert.Equal(t, "the answer", f.feed.frames[1].text)
	assert.False(t, f.feed.frames[1].startedAt.IsZero(), "an answer says when it began")
	assert.True(t, f.feed.frames[2].startedAt.IsZero())
	exit, err := f.r.ExitRows(f.state(t), testChat, "run-1")
	require.NoError(t, err)
	assert.Len(t, exit, 1, "what no row holds is the exit's to record")

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop})
	require.NoError(t, err)

	exit, err = f.r.ExitRows(f.state(t), testChat, "run-1")
	require.NoError(t, err)
	assert.Empty(t, exit, "the end recorded it")
	assert.Equal(t, []string{"the answer"}, said(f.rows(t)))
}

func TestRecorder_AnIdleThatEndsTheTurnKeepsItsStreamedText(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)
	_, _, err = f.dispatch(t, delta(engineagents.HookMessageDelta, "m1", false, false, "said"))
	require.NoError(t, err)

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookIdle})

	require.NoError(t, err)
	assert.Equal(t, []string{"said"}, said(f.rows(t)))
	exit, err := f.r.ExitRows(f.state(t), testChat, "run-1")
	require.NoError(t, err)
	assert.Empty(t, exit, "the end recorded it")
}

func TestRecorder_RetireAndForgetDropTheStreams(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)
	_, _, err = f.dispatch(t, delta(engineagents.HookMessageDelta, "m1", false, false, "x"))
	require.NoError(t, err)

	f.r.Retire(testChat, "run-1")
	exit, err := f.r.ExitRows(f.state(t), testChat, "run-1")
	require.NoError(t, err)
	assert.Empty(t, exit)

	f.r.ArmManualCompaction(testChat)
	f.r.ForgetChat(testChat)
	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookCompactPre})
	require.NoError(t, err)
	rows := f.rows(t)
	assert.Empty(t, bodyOf[ledger.Marker](t, rows[len(rows)-1]).Detail, "a forgotten chat's request is gone")
}

func TestRecorder_AManualCompactionIsMarkedAsAsked(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	f.r.ArmManualCompaction(testChat)

	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookCompactPre})

	require.NoError(t, err)
	rows := f.rows(t)
	assert.Equal(t, "manual", bodyOf[ledger.Marker](t, rows[len(rows)-1]).Detail)
}

func TestRecorder_TelemetryIsHeldAndPushed(t *testing.T) {
	f := newRecorderFixture(t, "claude")

	require.NoError(t, f.r.RecordTelemetry(t.Context(), f.runner, f.agent, []byte(`{}`)), "an empty report is no report")
	_, ok := f.r.Telemetry(testChat)
	assert.False(t, ok)

	require.NoError(t, f.r.RecordTelemetry(t.Context(), f.runner, f.agent,
		[]byte(`{"context_window":{"context_window_size":200000,"used_percentage":19},"model":{"id":"m","display_name":"M"}}`)))
	_, ok = f.r.Telemetry(testChat)
	assert.True(t, ok)
	assert.Equal(t, 1, f.feed.telemetry)

	err := f.r.RecordTelemetry(t.Context(), f.runner, f.agent, []byte(`not json`))
	require.ErrorIs(t, err, apperr.ErrUnprocessable)
	gone := f.runner
	gone.CurrentChatID = ""
	require.ErrorIs(t, f.r.RecordTelemetry(t.Context(), gone, f.agent, []byte(`{}`)), apperr.ErrNotFound)
}

func TestRecorder_DescriptorIsTheRunnersProviders(t *testing.T) {
	f := newRecorderFixture(t, "claude")

	agent, err := f.r.Descriptor(t.Context(), f.runner)
	require.NoError(t, err)
	assert.NotNil(t, agent)

	unknown := f.runner
	unknown.ProviderID = "no-such-provider"
	_, err = f.r.Descriptor(t.Context(), unknown)
	require.Error(t, err)

	f.r.ws = fakeWorkspace{err: errors.New("no workspace")}
	_, err = f.r.Descriptor(t.Context(), f.runner)
	require.ErrorContains(t, err, "no workspace")
}

func TestDeriveTitle_IsThePromptsFirstLine(t *testing.T) {
	assert.Equal(t, "first", deriveTitle("\n  \n first \nsecond"))
	assert.Empty(t, deriveTitle(" \n "))
	long := "a very long first line that runs well past the sixty characters a title may hold"
	assert.Equal(t, "a very long first line that runs well past the sixty charact…", deriveTitle(long))
}
