package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	arCmds "github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// frame is one hub broadcast the projections emitted, captured for assertion.
type frame struct {
	runnerID    string
	workspaceID string
	chatID      string
	kind        string
}

// harness drives the runner projections exactly as production does: by sending
// the agentrunner commands through asynx. There is no top-level agentrunner
// repository yet (it is a later task), so the commands are dispatched directly —
// which is also what the sibling agentchat store_test does.
//
// Synchronisation is on the REAL asynx signal, never on time: ax.SendWait
// blocks until every projection subscribed to the emitted event has finished,
// and drain() blocks on ax.WaitPublish (all async publishes + handlers
// complete). No sleeps, no polling, no timeouts anywhere in this file.
type harness struct {
	t      *testing.T
	ctx    context.Context
	ax     asynx.Asynx[domain.AgentRunner]
	st     *store.Store
	db     *gormdb.DB
	mu     sync.Mutex
	frames []frame
}

func newHarness(
	t *testing.T,
) *harness {
	t.Helper()
	return newHarnessWithEventStore(t, nil)
}

// newHarnessWithEventStore builds the harness with readES as the store's rebuild
// source. nil means "use the real event log the aggregate writes to".
func newHarnessWithEventStore(
	t *testing.T,
	readES asynxModels.Store,
) *harness {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRunner]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	if readES == nil {
		readES = es
	}
	h := &harness{t: t, ctx: context.Background(), ax: ax, db: db}
	st, err := store.New(db, readES, ax, h.record)
	require.NoError(t, err)
	h.st = st
	return h
}

func (h *harness) record(
	runnerID string,
	workspaceID string,
	chatID string,
	kind string,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames = append(h.frames, frame{runnerID: runnerID, workspaceID: workspaceID, chatID: chatID, kind: kind})
}

func (h *harness) kinds() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.frames))
	for _, f := range h.frames {
		out = append(out, f.kind)
	}
	return out
}

// drain blocks until every projection handler for every published event has
// run. It is the real asynx completion signal, not a wait.
func (h *harness) drain() {
	h.t.Helper()
	h.ax.WaitPublish()
}

func (h *harness) start(
	cmd arCmds.Start,
) {
	h.t.Helper()
	_, err := h.ax.SendWait(h.ctx, cmd)
	require.NoError(h.t, err)
}

func (h *harness) bindSession(
	runnerID string,
	sessionID string,
	now time.Time,
) {
	h.t.Helper()
	_, err := h.ax.SendWait(h.ctx, arCmds.BindSession{RunnerID: runnerID, SessionID: sessionID, Now: now})
	require.NoError(h.t, err)
}

func (h *harness) move(
	runnerID string,
	toChatID string,
	sessionID string,
	now time.Time,
) {
	h.t.Helper()
	_, err := h.ax.SendWait(h.ctx, arCmds.Move{RunnerID: runnerID, ToChatID: toChatID, SessionID: sessionID, Now: now})
	require.NoError(h.t, err)
}

func (h *harness) exit(
	runnerID string,
	now time.Time,
) {
	h.t.Helper()
	_, err := h.ax.SendWait(h.ctx, arCmds.Exit{RunnerID: runnerID, Now: now})
	require.NoError(h.t, err)
}

// A chat is "live" iff some runner points at it. It is NOT a stored flag.
func TestLiveness_FollowsTheRunner(t *testing.T) {
	h := newHarness(t)

	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.drain()

	live, err := h.st.LiveRunnerForChat(h.ctx, "c1")
	require.NoError(t, err)
	require.Equal(t, "r1", live.ID)
	assert.Equal(t, "pty1", live.TerminalSession)
	assert.Equal(t, "claude", live.ProviderID)

	// c2 is not live yet.
	_, err = h.st.LiveRunnerForChat(h.ctx, "c2")
	require.ErrorIs(t, err, store.ErrNotFound)
}

// THE test for the user's bug. The runner moves; the chat it LEFT goes dormant
// and the chat it ENTERED goes live — and neither chat aggregate was written.
func TestMove_TransfersLivenessWithoutWritingEitherChat(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.bindSession("r1", "s1", time.Unix(2, 0))
	h.move("r1", "c2", "s2", time.Unix(3, 0))
	h.drain()

	_, err := h.st.LiveRunnerForChat(h.ctx, "c1")
	require.ErrorIs(t, err, store.ErrNotFound, "the chat we left is dormant")

	live, err := h.st.LiveRunnerForChat(h.ctx, "c2")
	require.NoError(t, err)
	require.Equal(t, "r1", live.ID)
	require.Equal(t, "pty1", live.TerminalSession, "same PTY — the terminal never changed")
}

// The reducer's "is this id known?" question, and Resume's "where do I pick up?",
// both answered from append-only history.
func TestConversations_AreAppendOnlyHistory(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.bindSession("r1", "s1", time.Unix(2, 0))
	h.move("r1", "c2", "s2", time.Unix(3, 0))
	h.drain()

	chatID, err := h.st.ChatForSession(h.ctx, "w1", "s1")
	require.NoError(t, err)
	require.Equal(t, "c1", chatID, "s1 still belongs to c1 even though the runner left")

	chatID, err = h.st.ChatForSession(h.ctx, "w1", "s2")
	require.NoError(t, err)
	require.Equal(t, "c2", chatID)

	_, err = h.st.ChatForSession(h.ctx, "w1", "never-seen")
	require.ErrorIs(t, err, store.ErrNotFound)

	// Resume reads the tail.
	last, err := h.st.LastConversation(h.ctx, "c1")
	require.NoError(t, err)
	require.Equal(t, "claude", last.ProviderID)
	require.Equal(t, "s1", last.SessionID)
	assert.Equal(t, "c1", last.ChatID)
}

// Exit drops the liveness row but NOT the history: a dormant chat must still be
// resumable, and its conversation must still be recognised on a later /resume.
func TestExit_ClearsLivenessKeepsHistory(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.bindSession("r1", "s1", time.Unix(2, 0))
	h.exit("r1", time.Unix(4, 0))
	h.drain()

	_, err := h.st.LiveRunnerForChat(h.ctx, "c1")
	require.ErrorIs(t, err, store.ErrNotFound)

	chatID, err := h.st.ChatForSession(h.ctx, "w1", "s1")
	require.NoError(t, err)
	require.Equal(t, "c1", chatID, "history survives the process")

	last, err := h.st.LastConversation(h.ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "s1", last.SessionID, "a dormant chat stays resumable")

	// The runner itself is gone from the live model — there is no status column
	// left behind that could later disagree with the PTY.
	_, err = h.st.Get(h.ctx, "r1")
	require.ErrorIs(t, err, store.ErrNotFound)
}

// Invariant I3: at most one LIVE runner per conversation. This is what the
// eviction path queries.
func TestLiveRunnerForSession_FindsTheIncumbent(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r2", WorkspaceID: "w1", ProviderID: "codex",
		TerminalSession: "pty2", ChatID: "c2", Now: time.Unix(1, 0),
	})
	h.bindSession("r2", "s2", time.Unix(2, 0))
	h.drain()

	inc, err := h.st.LiveRunnerForSession(h.ctx, "w1", "s2")
	require.NoError(t, err)
	require.Equal(t, "r2", inc.ID)

	// A session nobody is currently running is not live, even in the same
	// workspace — and a live session in ANOTHER workspace is not ours.
	_, err = h.st.LiveRunnerForSession(h.ctx, "w1", "s-unknown")
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = h.st.LiveRunnerForSession(h.ctx, "w2", "s2")
	require.ErrorIs(t, err, store.ErrNotFound)
}

// Get resolves a live runner by id and reports ErrNotFound for one that never
// existed — the same absence-is-the-answer rule as LiveRunnerForChat.
func TestGet_ReturnsLiveRunnerOrNotFound(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.drain()

	got, err := h.st.Get(h.ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, "w1", got.WorkspaceID)
	assert.Equal(t, "c1", got.CurrentChatID)
	assert.Equal(t, time.Unix(1, 0).UTC(), got.StartedAt.UTC())

	_, err = h.st.Get(h.ctx, "nope")
	require.ErrorIs(t, err, store.ErrNotFound)
}

// AllLive feeds boot reconciliation: every runner the model believes is running,
// so the caller can ask the PTY — the sole authority — whether it really is.
func TestAllLive_ListsOnlyRunningRunners(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.start(arCmds.Start{
		RunnerID: "r2", WorkspaceID: "w1", ProviderID: "codex",
		TerminalSession: "pty2", ChatID: "c2", Now: time.Unix(2, 0),
	})
	h.exit("r1", time.Unix(3, 0))
	h.drain()

	live, err := h.st.AllLive(h.ctx)
	require.NoError(t, err)
	require.Len(t, live, 1, "the exited runner left no row behind")
	assert.Equal(t, "r2", live[0].ID)
}

// ForgetChat is the chat delete cascade — the ONLY thing allowed to remove
// append-only conversation history. It must not touch other chats' history.
func TestForgetChat_DropsOnlyThatChatsHistory(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.bindSession("r1", "s1", time.Unix(2, 0))
	h.move("r1", "c2", "s2", time.Unix(3, 0))
	h.drain()

	require.NoError(t, h.st.ForgetChat(h.ctx, "c1"))

	_, err := h.st.ChatForSession(h.ctx, "w1", "s1")
	require.ErrorIs(t, err, store.ErrNotFound, "the deleted chat's history is gone")
	_, err = h.st.LastConversation(h.ctx, "c1")
	require.ErrorIs(t, err, store.ErrNotFound)

	chatID, err := h.st.ChatForSession(h.ctx, "w1", "s2")
	require.NoError(t, err)
	assert.Equal(t, "c2", chatID, "a sibling chat's history is untouched")
}

// The hub projection turns each runner event into exactly one (runner, ws, chat,
// kind) frame, with the kind read off the event name.
func TestHub_BroadcastsOneFramePerEvent(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.bindSession("r1", "s1", time.Unix(2, 0))
	h.move("r1", "c2", "s2", time.Unix(3, 0))
	h.exit("r1", time.Unix(4, 0))
	h.drain()

	assert.Equal(t, []string{"started", "session_bound", "moved", "exited"}, h.kinds())

	h.mu.Lock()
	defer h.mu.Unlock()
	require.Len(t, h.frames, 4)
	assert.Equal(t, frame{runnerID: "r1", workspaceID: "w1", chatID: "c1", kind: "started"}, h.frames[0])
	assert.Equal(t, "c2", h.frames[2].chatID, "the moved frame carries the chat the runner entered")
}

// A lost read model heals from the event log: AllLive (boot reconciliation's
// read) replays every runner the log holds before concluding nothing is running.
func TestAllLive_RebuildsLostReadModel(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.bindSession("r1", "s1", time.Unix(2, 0))
	h.drain()

	require.NoError(t, h.db.WithContext(h.ctx).Exec("DELETE FROM agent_runners").Error)
	require.NoError(t, h.db.WithContext(h.ctx).Exec("DELETE FROM agent_chat_conversations").Error)

	live, err := h.st.AllLive(h.ctx)
	require.NoError(t, err)
	require.Len(t, live, 1)
	assert.Equal(t, "r1", live[0].ID)
	assert.Equal(t, "s1", live[0].CurrentSession)

	// The replay re-appended the conversation history too.
	chatID, err := h.st.ChatForSession(h.ctx, "w1", "s1")
	require.NoError(t, err)
	assert.Equal(t, "c1", chatID)
}

func TestAllLive_EmptyLogReturnsEmpty(t *testing.T) {
	h := newHarness(t)

	live, err := h.st.AllLive(h.ctx)
	require.NoError(t, err)
	assert.Empty(t, live)
}

// A genuine DB failure must NOT be mistaken for "dormant": every read has to
// surface it as an error, never as the ErrNotFound that would silently tell the
// caller a running CLI is gone.
func TestReads_SurfaceDBErrorsRatherThanReportingDormant(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.bindSession("r1", "s1", time.Unix(2, 0))
	h.drain()

	sqlDB, err := h.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = h.st.LiveRunnerForChat(h.ctx, "c1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, store.ErrNotFound)

	_, err = h.st.LiveRunnerForSession(h.ctx, "w1", "s1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, store.ErrNotFound)

	_, err = h.st.ChatForSession(h.ctx, "w1", "s1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, store.ErrNotFound)

	_, err = h.st.LastConversation(h.ctx, "c1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, store.ErrNotFound)

	_, err = h.st.Get(h.ctx, "r1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, store.ErrNotFound)

	_, err = h.st.AllLive(h.ctx)
	require.Error(t, err)

	require.Error(t, h.st.ForgetChat(h.ctx, "c1"))
}

// fakeES is an event store that cannot enumerate its aggregates. AllLive must
// then simply report the empty model rather than failing.
type fakeES struct {
	asynxModels.Store
}

func TestAllLive_EventStoreCannotEnumerate(t *testing.T) {
	h := newHarnessWithEventStore(t, &fakeES{})

	live, err := h.st.AllLive(h.ctx)
	require.NoError(t, err)
	assert.Empty(t, live)
}

// listerErrES enumerates aggregates but fails, so the rebuild's failure surfaces
// instead of being mistaken for "nothing is running".
type listerErrES struct {
	asynxModels.Store
	err error
}

func (f *listerErrES) AggregateIDs(context.Context) ([]string, error) {
	return nil, f.err
}

func TestAllLive_RebuildEnumerationError(t *testing.T) {
	h := newHarnessWithEventStore(t, &listerErrES{err: errors.New("boom")})

	_, err := h.st.AllLive(h.ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enumerate aggregate ids")
}

func TestNew_MigrationError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = store.New(db, nil, nil, func(string, string, string, string) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentrunner store")
}
