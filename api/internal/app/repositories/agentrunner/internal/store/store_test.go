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

// frameSink is the BroadcastFunc a Store is built with: it captures every hub
// frame the projections emit, so a test can assert both what was broadcast and
// what was NOT (a heal must broadcast nothing).
type frameSink struct {
	mu     sync.Mutex
	frames []frame
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
	ax   asynx.Asynx[domain.AgentRunner]
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
	st, err := store.New(db, readES, ax, h.sink.record)
	require.NoError(t, err)
	h.st = st
	return h
}

func newAx(
	t *testing.T,
	es asynxModels.Store,
) asynx.Asynx[domain.AgentRunner] {
	t.Helper()
	ax, err := asynx.New[domain.AgentRunner]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

// restart simulates a daemon restart that lost the read DB: a brand-new store
// over a brand-new read DB, on top of the SAME event log. Every PTY died with the
// old process, so nothing is running — which is exactly what the new store must
// conclude. sink captures anything the fresh store broadcasts while coming up.
func (h *harness) restart(
	sink *frameSink,
) *store.Store {
	h.t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(h.t, err)
	st, err := store.New(db, h.es, newAx(h.t, h.es), sink.record)
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

// Invariants I1/I3 say one live runner per chat and per conversation, and nothing
// in the schema enforces that (a unique index would turn eviction's transient
// states into projection write failures). So if the invariant is ever violated,
// the read must at least be DETERMINISTIC — lowest id wins, every time — or the
// resulting bug is unreproducible. rB is written first here; ordering by id, not
// by insertion, is what makes rA the answer.
func TestLiveReads_AreDeterministicWhenTheInvariantIsViolated(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "rB", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "ptyB", ChatID: "c1", Now: clock(10),
	})
	h.start(arCmds.Start{
		RunnerID: "rA", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "ptyA", ChatID: "c1", Now: clock(11),
	})
	h.bindSession("rB", "s1", clock(12))
	h.bindSession("rA", "s1", clock(13))
	h.drain()

	byChat, err := h.st.LiveRunnerForChat(h.ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "rA", byChat.ID, "two runners in one chat is a bug — but it must be the SAME bug on every read")

	bySession, err := h.st.LiveRunnerForSession(h.ctx, "w1", "s1")
	require.NoError(t, err)
	assert.Equal(t, "rA", bySession.ID)
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
	st := h.restart(sink)

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

// The heal runs at construction ONLY, and only into an empty history. A store
// coming up over an intact read DB never touches the event log — proven by giving
// it an event store whose enumeration would fail if it were ever asked.
func TestNew_DoesNotHealWhenHistoryIsIntact(t *testing.T) {
	h := newHarness(t)
	h.start(arCmds.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: clock(10),
	})
	h.bindSession("r1", "s1", clock(11))
	h.drain()

	_, err := store.New(h.db, &listerErrES{err: errors.New("boom")}, newAx(t, h.es), h.sink.record)
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
func TestNew_RejectsNilBroadcast(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	_, err = store.New(db, es, newAx(t, es), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil broadcast")
}

func noopBroadcast(
	_ string,
	_ string,
	_ string,
	_ string,
) {
}
