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
// what was NOT (a heal must broadcast nothing).
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
	es   asynxModels.Store
	sink *frameSink
}

func newHarness(
	t *testing.T,
) *harness {
	t.Helper()
	return newHarnessWithEventStore(t, nil)
}

// newHarnessWithEventStore builds the harness with readES as the store's heal
// source. nil means "use the real event log the aggregate writes to".
func newHarnessWithEventStore(
	t *testing.T,
	readES asynxModels.Store,
) *harness {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax := newAx(t, es)

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	if readES == nil {
		readES = es
	}
	h := &harness{t: t, ctx: context.Background(), ax: ax, db: db, es: es, sink: &frameSink{}}
	st, err := store.New(db, readES, ax, h.sink.watch)
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

// restart simulates an ORDINARY daemon restart: a new store, a new asynx, the
// same durable read DB and the same event log. This is the common case, and the
// read DB is intact — nothing needs healing. Every PTY died with the old process
// regardless.
func (h *harness) restart(
	sink *frameSink,
) *store.Store {
	h.t.Helper()
	st, err := store.New(h.db, h.es, newAx(h.t, h.es), sink.watch)
	require.NoError(h.t, err)
	return st
}

// restartLosingReadDB simulates the disaster case: the read DB is gone (deleted,
// corrupted, never existed) but the event log survives. A brand-new read DB on
// top of the SAME log — the one situation where history must be rebuilt.
func (h *harness) restartLosingReadDB(
	sink *frameSink,
) *store.Store {
	h.t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(h.t, err)
	st, err := store.New(db, h.es, newAx(h.t, h.es), sink.watch)
	require.NoError(h.t, err)
	return st
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

// clock renders "hour h of the same day" — the runner-spawn vs conversation-open
// distinction below is a story about hours apart, so read it as one.
func clock(
	hour int,
) time.Time {
	return time.Date(2026, time.July, 11, hour, 0, 0, 0, time.UTC)
}

// FirstSeenAt is when the CONVERSATION opened, not when the runner spawned. A
// long-lived runner opens conversations hours after it started; stamping them
// with its spawn time is a lie the frontend would render.
func TestConversation_FirstSeenAtIsWhenTheConversationOpened(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(10),
	})
	h.bindSession("r1", "s1", clock(11))
	h.move("r1", "c2", "s2", clock(13))
	h.drain()

	first, err := h.st.LastConversation(h.ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, clock(11).UTC(), first.FirstSeenAt.UTC(), "s1 opened when it was BOUND, not when r1 spawned")

	second, err := h.st.LastConversation(h.ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, clock(13).UTC(), second.FirstSeenAt.UTC(), "s2 opened when the runner MOVED into it")
}

// Two conversations in ONE chat — the case the ORDER BY exists for. The tail is
// the conversation opened LAST, which is where a resume must pick up.
func TestLastConversation_ReturnsTheLatestWithinAChat(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(10),
	})
	h.bindSession("r1", "s1", clock(11))
	h.move("r1", "c2", "s2", clock(12))
	h.move("r1", "c1", "s3", clock(13))
	h.drain()

	last, err := h.st.LastConversation(h.ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "s3", last.SessionID, "c1 hosted s1 then s3; the tail is s3")

	other, err := h.st.LastConversation(h.ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, "s2", other.SessionID)
}

// The cross-runner inversion. Two runners with different spawn times write into
// the SAME chat: ordering by the runner's spawn time returns the OLDER
// conversation, and a resume then reopens the chat at the wrong place.
func TestLastConversation_IsNotInvertedByAnOlderRunner(t *testing.T) {
	h := newHarness(t)

	// rA spawns at 10:00 in chat cA.
	h.start(arCmds.Start{
		RunnerID: "rA", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "ptyA", ChatID: "cA", Now: clock(10),
	})
	// rB spawns an hour later in cB and binds s1 there — then dies.
	h.start(arCmds.Start{
		RunnerID: "rB", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "ptyB", ChatID: "cB", Now: clock(11),
	})
	h.bindSession("rB", "s1", clock(11))
	h.exit("rB", clock(12))

	// The user /resumes s1 inside rA's PTY, so the OLDER runner moves into cB…
	h.move("rA", "cB", "s1", clock(13))
	// …and later opens a fresh conversation s2 there.
	h.move("rA", "cB", "s2", clock(14))
	h.drain()

	last, err := h.st.LastConversation(h.ctx, "cB")
	require.NoError(t, err)
	assert.Equal(t, "s2", last.SessionID, "the tail is the conversation opened last, not the one whose runner spawned last")
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

// Invariants I1/I3 (one live runner per chat, one per conversation) cannot hold at
// every INSTANT, and the read model must not pretend otherwise. Crowbar cannot kill a
// process synchronously — it SIGTERMs, and the runner does not die until its PTY does,
// because the PTY is the sole authority on liveness. So there is an unavoidable window
// where two rows point at one chat:
//
//	eviction: the mover takes the conversation; the incumbent is still dying.
//	switch:   the incoming CLI starts while the outgoing one is still dying.
//
// In both, the answer to "who holds this chat now" is THE ONE THAT ARRIVED LAST. The
// other is dying by our own hand. Ordering by id would be deterministic and WRONG — it
// would hand out the dying runner (and its dead PTY) about half the time.
//
// Here the low-id runner is the CORPSE: rA is the incumbent, rB the taker. A lowest-id
// read returns rA and fails.
func TestLiveReads_DuringEviction_ReturnTheRunnerThatArrivedLast(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "rA", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "ptyA", ChatID: "c1", Now: clock(10),
	})
	h.bindSession("rA", "s1", clock(11)) // rA holds conversation s1 in chat c1

	// rB is a DIFFERENT live CLI that /resumes into s1: it moves onto c1, and rA is
	// evicted — but rA's PTY has not died yet, so its row is still there.
	h.start(arCmds.Start{
		RunnerID: "rB", WorkspaceID: "w1", ProviderID: "codex",
		TerminalSession: "ptyB", ChatID: "c2", Now: clock(20),
	})
	h.move("rB", "c1", "s1", clock(30))
	h.drain()

	byChat, err := h.st.LiveRunnerForChat(h.ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "rB", byChat.ID, "the chat belongs to the runner that took it over, not the corpse")

	bySession, err := h.st.LiveRunnerForSession(h.ctx, "w1", "s1")
	require.NoError(t, err)
	assert.Equal(t, "rB", bySession.ID, "and so does the conversation")
}

// The same window on the provider-switch path: the incoming runner has only just
// STARTED (it has no conversation yet — CurrentSessionSince is zero), while the
// outgoing one still carries the conversation it bound long ago. The incoming one
// still arrived last, and the chat is its.
func TestLiveRunnerForChat_DuringSwitch_ReturnsTheIncomingRunner(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "outgoing", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty-old", ChatID: "c1", Now: clock(10),
	})
	h.bindSession("outgoing", "s1", clock(11))

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
	h.bindSession("r1", "s1", clock(2))
	h.drain()

	_, err := h.ax.SendWait(h.ctx, arCmds.Displace{RunnerID: "r1"})
	require.NoError(t, err)
	h.drain()

	_, err = h.st.LiveRunnerForChat(h.ctx, "c1")
	assert.ErrorIs(t, err, store.ErrNotFound, "the chat is dormant: nothing is on it")
	_, err = h.st.LiveRunnerForSession(h.ctx, "w1", "s1")
	assert.ErrorIs(t, err, store.ErrNotFound, "and nobody is holding the conversation")

	// The runner is still THERE, though: it is a live process, and only its PTY may say
	// otherwise. Boot reconciliation still finds it, and its exit still carries it away.
	got, err := h.st.Get(h.ctx, "r1")
	require.NoError(t, err)
	assert.Empty(t, got.CurrentChatID)
	assert.Equal(t, "pty1", got.TerminalSession)
	assert.Len(t, mustAllLive(t, h), 1, "a displaced runner is still a running CLI")

	// Its history survives: the chat it was on stays resumable.
	last, err := h.st.LastConversation(h.ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "s1", last.SessionID)
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

// LiveRunnersForSession is the I3 twin, and it exists for the same reason: once a placement
// has committed, the caller IS the newest holder, so the single-row read would hand it back
// its own row and it would evict nobody at all.
func TestLiveRunnersForSession_ReturnsEveryHolderNewestFirst(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "holder", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(10),
	})
	h.bindSession("holder", "s1", clock(11))

	// A second CLI announces the SAME conversation (a --continue-style descriptor override is
	// all it takes): two runners on one provider session id, which is the I3 violation.
	h.start(arCmds.Start{
		RunnerID: "newcomer", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty2", ChatID: "c2", Now: clock(20),
	})
	h.bindSession("newcomer", "s1", clock(21))
	h.drain()

	got, err := h.st.LiveRunnersForSession(h.ctx, "w1", "s1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "newcomer", got[0].ID, "newest arrival first — the one that must survive")
	assert.Equal(t, "holder", got[1].ID)

	// Scoped to its workspace, and nowhere is held by nobody.
	got, err = h.st.LiveRunnersForSession(h.ctx, "other-ws", "s1")
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = h.st.LiveRunnersForSession(h.ctx, "w1", "")
	require.NoError(t, err)
	assert.Empty(t, got)
}

// "" is NOWHERE, not a key. A displaced runner's row carries an empty chat id and an empty
// session, so an unguarded lookup would MATCH those rows — handing a caller a runner that is
// on nothing, which is the read model volunteering a lie.
func TestLiveReads_EmptyKeyIsNowhere(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(1),
	})
	h.bindSession("r1", "s1", clock(2))
	_, err := h.ax.SendWait(h.ctx, arCmds.Displace{RunnerID: "r1"})
	require.NoError(t, err)
	h.drain()

	_, err = h.st.LiveRunnerForChat(h.ctx, "")
	assert.ErrorIs(t, err, store.ErrNotFound, "an empty chat id must never match a displaced runner")

	_, err = h.st.LiveRunnerForSession(h.ctx, "w1", "")
	assert.ErrorIs(t, err, store.ErrNotFound, "nor an empty session id")
}

// ConversationsForChat is the append-only history a provider switch reads to find the
// conversation the INCOMING provider left behind here. LastConversation cannot answer
// it: after a handoff the chat's newest conversation belongs to the provider being
// switched AWAY from.
func TestConversationsForChat_ReturnsEveryConversationOldestFirst(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(1),
	})
	h.bindSession("r1", "s-claude", clock(2))
	h.exit("r1", clock(3))

	// The chat is handed to codex: a second runner, a second conversation, same chat.
	h.start(arCmds.Start{
		RunnerID: "r2", WorkspaceID: "w1", ProviderID: "codex",
		TerminalSession: "pty2", ChatID: "c1", Now: clock(4),
	})
	h.bindSession("r2", "s-codex", clock(5))

	// A conversation in ANOTHER chat must never leak in.
	h.start(arCmds.Start{
		RunnerID: "r3", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty3", ChatID: "c2", Now: clock(6),
	})
	h.bindSession("r3", "s-other", clock(7))
	h.drain()

	convs, err := h.st.ConversationsForChat(h.ctx, "c1")
	require.NoError(t, err)
	require.Len(t, convs, 2)
	assert.Equal(t, "s-claude", convs[0].SessionID, "oldest first")
	assert.Equal(t, "claude", convs[0].ProviderID)
	assert.Equal(t, "s-codex", convs[1].SessionID)
	assert.Equal(t, "codex", convs[1].ProviderID)

	// Its history outlived the runner that opened it (r1 exited) — that is the whole
	// point of history: a dormant chat stays resumable.
	assert.Equal(t, "c1", convs[0].ChatID)
}

func TestConversationsForChat_UnknownChatIsEmptyNotAnError(t *testing.T) {
	h := newHarness(t)
	convs, err := h.st.ConversationsForChat(h.ctx, "never-existed")
	require.NoError(t, err)
	assert.Empty(t, convs)
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

	assert.Equal(t, []string{"started", "session_bound", "moved", "exited"}, h.sink.kinds())

	frames := h.sink.all()
	require.Len(t, frames, 4)
	assert.Equal(t, frame{runnerID: "r1", workspaceID: "w1", chatID: "c1", kind: "started"}, frames[0])
	assert.Equal(t, "c2", frames[2].chatID, "the moved frame carries the chat the runner entered")
}

// THE heal test. A restart loses the read DB but not the event log. History is
// durable truth and must come back — the live-runner table is NOT, and must not:
// every PTY died with the old process, so replaying a never-exited runner into
// agent_runners would manufacture the exact "live row, dead CLI" drift this
// package exists to make unrepresentable.
func TestNew_HealsHistoryAndNeverResurrectsRunners(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(10),
	})
	h.bindSession("r1", "s1", clock(11))
	h.move("r1", "c2", "s2", clock(12))
	h.drain()

	// r1 never exited: the log's last word on it is "running in c2".
	sink := &frameSink{}
	st := h.restartLosingReadDB(sink)

	chatID, err := st.ChatForSession(h.ctx, "w1", "s1")
	require.NoError(t, err)
	assert.Equal(t, "c1", chatID, "history is durable truth — lose it and the chat is unresumable")
	last, err := st.LastConversation(h.ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, "s2", last.SessionID)
	assert.Equal(t, clock(12).UTC(), last.FirstSeenAt.UTC(), "the heal preserves WHEN each conversation opened")

	live, err := st.AllLive(h.ctx)
	require.NoError(t, err)
	assert.Empty(t, live, "the PTY did not survive the restart, so no runner is live")
	_, err = st.LiveRunnerForChat(h.ctx, "c2")
	require.ErrorIs(t, err, store.ErrNotFound, "a live row for a dead CLI is the drift being deleted")
	_, err = st.Get(h.ctx, "r1")
	require.ErrorIs(t, err, store.ErrNotFound)

	assert.Empty(t, sink.all(), "a heal folds with a bare projector — it must not re-broadcast historical frames")
}

// A DELETED chat must STAY deleted across a boot. The runner event log is never
// Forgotten — it keeps every (chat, session) pair forever — so a heal that
// triggers on "the conversation table is empty" resurrects the history of every
// hard-deleted chat the moment the user deletes their last chat. ChatForSession
// would then resolve a session to a chat id that no longer exists: the dangling
// chat on /resume that the heal exists to PREVENT, reintroduced by the heal.
//
// "Empty" and "emptied on purpose" are different facts. Only the marker can tell
// them apart.
func TestForgetChat_IsNotUndoneByTheNextBoot(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(10),
	})
	h.bindSession("r1", "s1", clock(11))
	h.move("r1", "c2", "s2", clock(12))
	h.exit("r1", clock(13))
	h.drain()

	// The user deletes both chats — every chat they had. The conversation table is
	// now empty, and the event log still remembers all of it.
	require.NoError(t, h.st.ForgetChat(h.ctx, "c1"))
	require.NoError(t, h.st.ForgetChat(h.ctx, "c2"))

	st := h.restart(&frameSink{})

	_, err := st.ChatForSession(h.ctx, "w1", "s1")
	require.ErrorIs(t, err, store.ErrNotFound, "a deleted chat's conversations stay dead")
	_, err = st.ChatForSession(h.ctx, "w1", "s2")
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = st.LastConversation(h.ctx, "c1")
	require.ErrorIs(t, err, store.ErrNotFound, "nothing may answer for a chat that no longer exists")
	_, err = st.LastConversation(h.ctx, "c2")
	require.ErrorIs(t, err, store.ErrNotFound)
}

// failConversationWrites makes every conversation-row INSERT fail the way a
// transient sqlite "disk I/O error" does — the failure agentchat's store already
// carries a retry for, so not a hypothetical. Registered as a gorm callback so it
// can be LIFTED again, which is how the next boot gets to succeed.
func failConversationWrites(
	t *testing.T,
	db *gormdb.DB,
) {
	t.Helper()
	err := db.Callback().Create().Before("gorm:create").Register(failCallback, func(tx *gormdb.DB) {
		if tx.Statement.Table == "agent_chat_conversations" {
			_ = tx.AddError(errors.New("disk I/O error"))
		}
	})
	require.NoError(t, err)
}

const failCallback = "test:fail_conversation_writes"

func markerCount(
	t *testing.T,
	db *gormdb.DB,
) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM agent_runner_heal_marker").Scan(&count).Error)
	return count
}

// A heal that could not write every row must NOT be recorded as done. The fold
// logs its write errors (the LIVE projection must never take the daemon down), so
// a transient failure would otherwise lose those conversations, return nil, and
// mark the read model built — permanently, never retried. That is the same
// half-a-history the enumerate path hard-errors on.
//
// The heal fold is a separate type from the live fold precisely so it can be
// strict where the live projection must not be.
func TestNew_DoesNotMarkBuiltWhenTheHealCannotWrite(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(10),
	})
	h.bindSession("r1", "s1", clock(11))
	h.move("r1", "c2", "s2", clock(12))
	h.drain()

	// Boot onto a lost read DB whose conversation writes are transiently failing.
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	failConversationWrites(t, db)

	_, err = store.New(db, h.es, newAx(t, h.es), noopBroadcast)
	require.Error(t, err, "a heal that lost rows is a failed heal")
	assert.Zero(t, markerCount(t, db), "and it must not be recorded as built, or it is never retried")

	// The I/O blip passes. The next boot re-heals — safe, because the append is
	// idempotent on the composite key — and this time the history comes back whole.
	require.NoError(t, db.Callback().Create().Remove(failCallback))

	st, err := store.New(db, h.es, newAx(t, h.es), noopBroadcast)
	require.NoError(t, err)
	assert.Equal(t, int64(1), markerCount(t, db))

	chatID, err := st.ChatForSession(h.ctx, "w1", "s1")
	require.NoError(t, err)
	assert.Equal(t, "c1", chatID, "the retried heal completed the history")
	last, err := st.LastConversation(h.ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, "s2", last.SessionID)
}

// The heal runs at construction ONLY, and only when this read DB has never been
// built before. A store coming up over a read DB whose MARKER is present never
// touches the event log — proven by giving it an event store whose enumeration
// would fail if it were ever asked.
func TestNew_DoesNotHealWhenTheMarkerIsPresent(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(10),
	})
	h.bindSession("r1", "s1", clock(11))
	h.drain()

	_, err := store.New(h.db, &listerErrES{err: errors.New("boom")}, newAx(t, h.es), h.sink.watch)
	require.NoError(t, err, "the event log was never enumerated: the history was already there")
}

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

// fakeES is an event store that cannot enumerate its aggregates. The heal is
// best-effort on that capability: the store still comes up.
type fakeES struct {
	asynxModels.Store
}

func TestNew_EventStoreCannotEnumerate(t *testing.T) {
	h := newHarnessWithEventStore(t, &fakeES{})

	live, err := h.st.AllLive(h.ctx)
	require.NoError(t, err)
	assert.Empty(t, live)
}

// listerErrES enumerates aggregates but fails, so a heal that CAN run but breaks
// surfaces at construction instead of booting on half a history — which is how a
// later /resume would mint a duplicate chat.
type listerErrES struct {
	asynxModels.Store
	err error
}

func (f *listerErrES) AggregateIDs(context.Context) ([]string, error) {
	return nil, f.err
}

func TestNew_HealEnumerationError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	_, err = store.New(db, &listerErrES{err: errors.New("boom")}, newAx(t, es), noopBroadcast)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enumerate aggregate ids")
}

func TestNew_MigrationError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = store.New(db, nil, nil, noopBroadcast)
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

	_, err = store.New(db, es, newAx(t, es), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil watch")
}

func noopBroadcast(_ store.RunnerEvent) {}
