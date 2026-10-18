package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	arCmds "github.com/char2cs/crowbar/api/internal/engine/agents/runner/internal/commands"
	"github.com/char2cs/crowbar/api/internal/engine/agents/runner/internal/store"
)

// frame is one hub broadcast the projections emitted, captured for assertion.
type frame struct {
	runnerID    string
	workspaceID string
	chatID      string
	kind        string
}

// frameSink is the BroadcastFunc a Store is built with: it captures every hub
// frame the projections emit, so a test can assert both what was broadcast and
// what was NOT.
type frameSink struct {
	mu     sync.Mutex
	frames []frame
}

// watch adapts the repository's announcement seam onto this sink's existing frame
// recorder, so the change is proven against the assertions already here.
func (s *frameSink) watch(e store.RunnerEvent) {
	s.record(e.RunnerID, e.WorkspaceID, e.ChatID, e.Kind)
}

func (s *frameSink) record(
	runnerID string,
	workspaceID string,
	chatID string,
	kind string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames = append(s.frames, frame{runnerID: runnerID, workspaceID: workspaceID, chatID: chatID, kind: kind})
}

func (s *frameSink) all() []frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]frame(nil), s.frames...)
}

func (s *frameSink) kinds() []string {
	out := make([]string, 0)
	for _, f := range s.all() {
		out = append(out, f.kind)
	}
	return out
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
	t    *testing.T
	ctx  context.Context
	ax   asynx.Asynx[agents.Runner]
	st   *store.Store
	db   *gormdb.DB
	sink *frameSink
}

func newHarness(
	t *testing.T,
) *harness {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax := newAx(t, es)

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	h := &harness{t: t, ctx: context.Background(), ax: ax, db: db, sink: &frameSink{}}
	st, err := store.New(db, ax, h.sink.watch)
	require.NoError(t, err)
	h.st = st
	return h
}

func newAx(
	t *testing.T,
	es asynxModels.Store,
) asynx.Asynx[agents.Runner] {
	t.Helper()
	ax, err := asynx.New[agents.Runner]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
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

// clock renders "hour h of the same day" — the runner-spawn vs conversation-open
// distinction below is a story about hours apart, so read it as one.
func clock(
	hour int,
) time.Time {
	return time.Date(2026, time.July, 11, hour, 0, 0, 0, time.UTC)
}

func TestExit_ClearsLivenessKeepsHistory(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	h.exit("r1", time.Unix(4, 0))
	h.drain()

	_, err := h.st.LiveRunnerForChat(h.ctx, "c1")
	require.ErrorIs(t, err, store.ErrNotFound)

	placed, err := h.st.PlacementsForChat(h.ctx, "c1")
	require.NoError(t, err)
	require.Len(t, placed, 1, "history survives the process")

	// The runner itself is gone from the live model — there is no status column
	// left behind that could later disagree with the PTY.
	_, err = h.st.Get(h.ctx, "r1")
	require.ErrorIs(t, err, store.ErrNotFound)
}

// The provider-switch window: the incoming runner started while the outgoing one is
// still dying. The incoming one arrived last, and the chat is its.
func TestLiveRunnerForChat_DuringSwitch_ReturnsTheIncomingRunner(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "outgoing", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty-old", ChatID: "c1", Now: clock(10),
	})

	// The switch: the new CLI is spawned while the old one is still dying.
	h.start(arCmds.Start{
		RunnerID: "incoming", WorkspaceID: "w1", ProviderID: "codex",
		TerminalSession: "pty-new", ChatID: "c1", Now: clock(20),
	})
	h.drain()

	live, err := h.st.LiveRunnerForChat(h.ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "incoming", live.ID, "the pane must attach to the incoming CLI's PTY, never the dying one")
	assert.Equal(t, "pty-new", live.TerminalSession)
}

// Displacement is how I2/I3 hold at every INSTANT rather than eventually. The runner is
// still RUNNING — its row survives, its PTY is its own — it is simply no longer on any
// chat, which is a fact Crowbar owns outright and can therefore record the moment it
// decides it, without waiting for a process it does not command to die.
func TestDisplace_ClearsPlacementButKeepsTheLiveRow(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(1),
	})
	h.drain()

	_, err := h.ax.SendWait(h.ctx, arCmds.Displace{RunnerID: "r1"})
	require.NoError(t, err)
	h.drain()

	_, err = h.st.LiveRunnerForChat(h.ctx, "c1")
	assert.ErrorIs(t, err, store.ErrNotFound, "the chat is dormant: nothing is on it")

	// The runner is still THERE, though: it is a live process, and only its PTY may say
	// otherwise. Boot reconciliation still finds it, and its exit still carries it away.
	got, err := h.st.Get(h.ctx, "r1")
	require.NoError(t, err)
	assert.Empty(t, got.CurrentChatID)
	assert.Equal(t, "pty1", got.TerminalSession)
	assert.Len(t, mustAllLive(t, h), 1, "a displaced runner is still a running CLI")
}

// mustAllLive reads the live-runner table.
func mustAllLive(t *testing.T, h *harness) []agents.Runner {
	t.Helper()
	rows, err := h.st.AllLive(h.ctx)
	require.NoError(t, err)
	return rows
}

// LiveRunnersForChat is the read the two PLACEMENT paths use: their job is to leave exactly
// ONE runner on the chat, and a single-row read cannot tell "nobody else" from "somebody
// else, and maybe more". It returns everyone, newest arrival first, so the caller can retire
// all but itself.
func TestLiveRunnersForChat_ReturnsEveryoneOnTheChatNewestFirst(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "old", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty-old", ChatID: "c1", Now: clock(10),
	})
	h.start(arCmds.Start{
		RunnerID: "new", WorkspaceID: "w1", ProviderID: "codex",
		TerminalSession: "pty-new", ChatID: "c1", Now: clock(20),
	})
	// A runner on a DIFFERENT chat must never leak in.
	h.start(arCmds.Start{
		RunnerID: "elsewhere", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty-x", ChatID: "c2", Now: clock(30),
	})
	h.drain()

	got, err := h.st.LiveRunnersForChat(h.ctx, "c1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "new", got[0].ID, "newest arrival first")
	assert.Equal(t, "old", got[1].ID)

	// A displaced runner is on NO chat, so it is nobody's problem to evict.
	_, err = h.ax.SendWait(h.ctx, arCmds.Displace{RunnerID: "old"})
	require.NoError(t, err)
	h.drain()

	got, err = h.st.LiveRunnersForChat(h.ctx, "c1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "new", got[0].ID)

	// Nowhere holds nobody.
	got, err = h.st.LiveRunnersForChat(h.ctx, "")
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = h.st.LiveRunnersForChat(h.ctx, "never-existed")
	require.NoError(t, err)
	assert.Empty(t, got)
}

// "" is NOWHERE, not a key. A displaced runner's row carries an empty chat id, so an
// unguarded lookup would MATCH those rows — handing a caller a runner that is
// on nothing, which is the read model volunteering a lie.
func TestLiveReads_EmptyKeyIsNowhere(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(1),
	})
	_, err := h.ax.SendWait(h.ctx, arCmds.Displace{RunnerID: "r1"})
	require.NoError(t, err)
	h.drain()

	_, err = h.st.LiveRunnerForChat(h.ctx, "")
	assert.ErrorIs(t, err, store.ErrNotFound, "an empty chat id must never match a displaced runner")
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

func TestForgetChat_DropsOnlyThatChatsHistory(t *testing.T) {
	h := newHarness(t)
	for _, chat := range []string{"c1", "c2"} {
		h.start(arCmds.Start{
			RunnerID: "r-" + chat, WorkspaceID: "w1", ProviderID: "claude",
			TerminalSession: "pty-" + chat, ChatID: chat, Now: time.Unix(1, 0),
		})
	}
	h.drain()

	require.NoError(t, h.st.ForgetChat(h.ctx, "c1"))

	gone, err := h.st.PlacementsForChat(h.ctx, "c1")
	require.NoError(t, err)
	assert.Empty(t, gone, "the deleted chat's history is gone")
	kept, err := h.st.PlacementsForChat(h.ctx, "c2")
	require.NoError(t, err)
	assert.Len(t, kept, 1, "a sibling chat's history is untouched")
}

// The hub projection turns each runner event into exactly one (runner, ws, chat,
// kind) frame, with the kind read off the event name.
func TestHub_BroadcastsOneFramePerEvent(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	_, err := h.ax.SendWait(h.ctx, arCmds.Report{RunnerID: "r1", Model: "m"})
	require.NoError(t, err)
	h.exit("r1", time.Unix(4, 0))
	h.drain()

	assert.Equal(t, []string{"started", "session_bound", "exited"}, h.sink.kinds())

	frames := h.sink.all()
	require.Len(t, frames, 3)
	assert.Equal(t, frame{runnerID: "r1", workspaceID: "w1", chatID: "c1", kind: "started"}, frames[0])
}

const failCallback = "test:fail_conversation_writes"

// An idle machine is the steady state, not a symptom: an empty live table is a
// real answer, and reading it must not drag the whole event log through a replay.
func TestAllLive_EmptyTableIsTheAnswer(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(10),
	})
	h.exit("r1", clock(11))
	h.drain()

	live, err := h.st.AllLive(h.ctx)
	require.NoError(t, err)
	assert.Empty(t, live, "every runner has exited — nothing is running, and no read heals that away")
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
	h.drain()

	sqlDB, err := h.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = h.st.LiveRunnerForChat(h.ctx, "c1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, store.ErrNotFound)

	_, err = h.st.Get(h.ctx, "r1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, store.ErrNotFound)

	_, err = h.st.AllLive(h.ctx)
	require.Error(t, err)

	require.Error(t, h.st.ForgetChat(h.ctx, "c1"))
}

func TestNew_MigrationError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = store.New(db, nil, noopBroadcast)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentrunner store")
}

// A nil broadcast must be refused at construction: the hub projection calls it on
// every event, so accepting one defers the panic into a projection goroutine,
// long after — and far from — whoever built the Store.
// A runner store with no watch seam silently loses every lifecycle frame, so a nil
// one is refused at construction rather than degraded to a no-op. agentchat differs
// deliberately: most of its callers are tests with no hub.
func TestNew_RejectsNilWatch(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	_, err = store.New(db, newAx(t, es), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil watch")
}

func noopBroadcast(_ store.RunnerEvent) {}

// subscribeFailsOnCall wraps a real Asynx and refuses its Nth Subscribe call,
// so a test can force EITHER of New's two Subscribe-registering steps to fail
// without needing a real asynx failure mode. New calls registerStoreProjection
// before registerHubProjection, so failOn:1 targets the first and failOn:2 the
// second — proving each one's OWN error is what New surfaces, not just "some
// error happened".
type subscribeFailsOnCall struct {
	asynx.Asynx[agents.Runner]
	failOn int
	calls  int
}

func (f *subscribeFailsOnCall) Subscribe(
	pattern string,
	handler asynxModels.ProjectionHandler[agents.Runner],
	opts ...asynxModels.SubscriptionOpt[agents.Runner],
) (string, error) {
	f.calls++
	if f.calls == f.failOn {
		return "", errors.New("bus refused the subscription")
	}
	return f.Asynx.Subscribe(pattern, handler, opts...)
}

// TestNew_StoreProjectionSubscribeFailurePropagates pins that a failure
// registering the READ-MODEL projection aborts construction with ITS OWN
// error, not a generic one — registerStoreProjection is the first of New's two
// Subscribe calls.
func TestNew_StoreProjectionSubscribeFailurePropagates(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	_, err = store.New(db, &subscribeFailsOnCall{Asynx: newAx(t, es), failOn: 1}, noopBroadcast)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentrunner store projection: subscribe:",
		"the failure must be attributed to the read-model projection specifically")
}

// TestNew_HubProjectionSubscribeFailurePropagates is the sibling for the SECOND
// Subscribe call: the read-model projection registers fine, and only the hub
// (WS fan-out) projection fails — pinning that New still surfaces it, rather
// than treating registerStoreProjection's success as construction being done.
func TestNew_HubProjectionSubscribeFailurePropagates(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	_, err = store.New(db, &subscribeFailsOnCall{Asynx: newAx(t, es), failOn: 2}, noopBroadcast)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentrunner hub projection: subscribe:",
		"the failure must be attributed to the hub projection, reached only after the read-model one succeeded")
}
