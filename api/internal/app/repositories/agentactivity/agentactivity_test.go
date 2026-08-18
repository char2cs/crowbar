package agentactivity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const chat = "chat-1"

var t0 = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)

type fixture struct {
	repo agentactivity.EventStore
	wait func()
	db   *gormdb.DB
	ax   asynx.Asynx[domain.AgentActivity]
	es   asynxModels.Store
	dir  string
	ctx  context.Context
}

// newFixture builds the REAL repository over an in-memory event log, read model
// and content directory: the real commands, the real projection, the real store.
func newFixture(t *testing.T) fixture {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentActivity]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	dir := t.TempDir()

	repo, err := agentactivity.NewEventSourced(ax, es, db, dir)
	require.NoError(t, err)
	return fixture{repo: repo, wait: ax.WaitPublish, db: db, ax: ax, es: es, dir: dir, ctx: context.Background()}
}

func (f fixture) turn(t *testing.T, id, role, text string, at time.Time) {
	t.Helper()
	require.NoError(t, f.repo.AppendTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: id, Role: role, ProviderID: "claude",
		RunnerID: "r1", SessionID: "s1", Text: text, Now: at,
	}))
}

func TestAppendTurn_IsReadableImmediately(t *testing.T) {
	f := newFixture(t)

	f.turn(t, "t1", domain.TurnRoleUser, "hello", t0)

	turns, err := f.repo.Turns(f.ctx, chat, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, turns, 1)
	assert.Equal(t, "hello", turns[0].Text)
	assert.Equal(t, "claude", turns[0].ProviderID)
}

// The turn commands take the barrier path precisely so a caller reading straight
// back — the chat surface, the handoff assembler, the delivery recovery — cannot
// see a lagging model.
func TestTurns_AreOrderedBySequenceNotByClock(t *testing.T) {
	f := newFixture(t)

	// Same wall-clock instant for all three: two hooks can land in the same
	// millisecond, and a conversation that renders out of order is worse than one
	// that renders late.
	f.turn(t, "t1", domain.TurnRoleUser, "first", t0)
	f.turn(t, "t2", domain.TurnRoleAssistant, "second", t0)
	f.turn(t, "t3", domain.TurnRoleUser, "third", t0)

	turns, err := f.repo.Turns(f.ctx, chat, 0, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second", "third"}, textsOf(turns))
	assert.Less(t, turns[0].Seq, turns[1].Seq)
}

// The projection upserts by the item's own id, so a redelivered hook rewrites the
// same row instead of duplicating the turn. Without it, "the daemon did not answer
// in time" and "the user said it twice" would be indistinguishable.
func TestAppendTurn_IsIdempotentOnItsTurnID(t *testing.T) {
	f := newFixture(t)

	f.turn(t, "delivery-1", domain.TurnRoleUser, "only once", t0)
	f.turn(t, "delivery-1", domain.TurnRoleUser, "only once", t0)

	turns, err := f.repo.Turns(f.ctx, chat, 0, 0, 0)
	require.NoError(t, err)
	assert.Len(t, turns, 1)
}

// Providers guarantee ids unique within a session, not across every chat this
// daemon has ever hosted.
func TestTurns_AreScopedToTheirChat(t *testing.T) {
	f := newFixture(t)
	f.turn(t, "shared-id", domain.TurnRoleUser, "chat one", t0)
	require.NoError(t, f.repo.AppendTurn(f.ctx, agentactivity.TurnInput{
		ChatID: "chat-2", TurnID: "shared-id", Role: domain.TurnRoleUser, Text: "chat two", Now: t0,
	}))

	one, err := f.repo.Turns(f.ctx, chat, 0, 0, 0)
	require.NoError(t, err)
	two, err := f.repo.Turns(f.ctx, "chat-2", 0, 0, 0)
	require.NoError(t, err)

	assert.Equal(t, []string{"chat one"}, textsOf(one))
	assert.Equal(t, []string{"chat two"}, textsOf(two))
}

func TestToolCall_RoundTripsThroughInvokeAndComplete(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.repo.OpenTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: t0,
	}))

	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: chat, ToolID: "tool-1", Name: "Edit", Target: "a.go",
		Request: []byte(`{"file_path":"a.go"}`), Now: t0,
	}))
	require.NoError(t, f.repo.CompleteTool(f.ctx, agentactivity.ToolResultInput{
		ChatID: chat, ToolID: "tool-1", Result: []byte("applied"),
		Status: domain.ToolStatusOK, DurationMS: 12, Now: t0.Add(time.Second),
	}))
	f.wait()

	calls, err := f.repo.ToolCalls(f.ctx, chat, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, "Edit", calls[0].Name)
	assert.Equal(t, "a.go", calls[0].Target)
	assert.Equal(t, domain.ToolStatusOK, calls[0].Status)
	assert.Equal(t, 12, calls[0].DurationMS)
	assert.Equal(t, "t1", calls[0].TurnID)

	// The payloads are addressed, never inlined.
	request, err := f.repo.Payload(f.ctx, calls[0].RequestRef)
	require.NoError(t, err)
	assert.JSONEq(t, `{"file_path":"a.go"}`, string(request))
	result, err := f.repo.Payload(f.ctx, calls[0].ResultRef)
	require.NoError(t, err)
	assert.Equal(t, "applied", string(result))
}

// A running call is visible while it runs, or the UI cannot say "Bash, 12s".
func TestInvokeTool_IsVisibleBeforeItCompletes(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: chat, ToolID: "tool-1", Name: "Bash", Now: t0,
	}))
	f.wait()

	calls, err := f.repo.ToolCalls(f.ctx, chat, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, domain.ToolStatusRunning, calls[0].Status)
}

// Providers do not guarantee a completion for every invocation. "Running for
// three days" is a worse lie than "abandoned".
func TestCloseTurn_AbandonsToolsWhoseCompletionNeverArrived(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.repo.OpenTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", Now: t0,
	}))
	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: chat, ToolID: "orphan", Name: "Bash", Now: t0,
	}))
	f.wait()

	require.NoError(t, f.repo.CloseTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", Text: "done", Now: t0.Add(time.Minute),
	}))

	calls, err := f.repo.ToolCalls(f.ctx, chat, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, domain.ToolStatusAbandoned, calls[0].Status)
	assert.NotNil(t, calls[0].EndedAt)
}

func TestSubagentsAndInterruptions_AreRecorded(t *testing.T) {
	f := newFixture(t)

	require.NoError(t, f.repo.StartSubagent(f.ctx, chat, "a1", "explore", t0))
	require.NoError(t, f.repo.StopSubagent(f.ctx, chat, "a1", "explore", t0.Add(time.Second)))
	require.NoError(t, f.repo.Interrupt(f.ctx, chat, "i1", "permission", "Bash", t0))
	require.NoError(t, f.repo.ResolveInterruption(f.ctx, chat, "i1", "permission", "Bash", t0.Add(time.Second)))
	f.wait()

	subs, err := f.repo.Subagents(f.ctx, chat)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "explore", subs[0].AgentType)
	assert.NotNil(t, subs[0].EndedAt)

	ints, err := f.repo.Interruptions(f.ctx, chat)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, "permission", ints[0].Kind)
	assert.NotNil(t, ints[0].ResolvedAt)
}

// A turn that ended with nothing said is not a message. It happens on every
// prompt submission — the outgoing CLI is replaced mid-turn — and a blank
// assistant row would read as the agent having answered with silence.
func TestAbandon_ClosesAnOpenTurnWithoutRecordingABlankReply(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.repo.OpenTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: t0,
	}))
	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: chat, ToolID: "tool-1", Name: "Bash", Now: t0,
	}))
	f.wait()

	require.NoError(t, f.repo.Abandon(f.ctx, chat, t0.Add(time.Minute)))

	turns, err := f.repo.Turns(f.ctx, chat, 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, turns, "an abandoned turn said nothing, so it is not a message")

	// What it DID is still closed out, so nothing renders as running forever.
	calls, err := f.repo.ToolCalls(f.ctx, chat, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, domain.ToolStatusAbandoned, calls[0].Status)
}

// Verified live against claude 2.1.233 on 2026-08-17: a Notification fired
// during a turn and stayed OPEN for the rest of the chat, because neither
// provider has an event that resolves one. Rendered, that is a permanent "the
// agent needs your attention" banner over an agent that is perfectly fine.
func TestRegression_AnInterruptionDoesNotOutliveItsTurn(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.repo.OpenTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: t0,
	}))
	require.NoError(t, f.repo.Interrupt(f.ctx, chat, "i1", "notification", "needs you", t0))
	f.wait()

	open, err := f.repo.Interruptions(f.ctx, chat)
	require.NoError(t, err)
	require.Len(t, open, 1)
	require.Nil(t, open[0].ResolvedAt, "precondition: it is open while the turn runs")

	require.NoError(t, f.repo.CloseTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", Text: "done", Now: t0.Add(time.Second),
	}))

	after, err := f.repo.Interruptions(f.ctx, chat)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.NotNil(t, after[0].ResolvedAt,
		"a notification has no resolving event of its own; the turn boundary is what ends it")
}

// The same holds when the turn is abandoned rather than answered — which is what
// a prompt submission does to the outgoing CLI.
func TestRegression_AnInterruptionIsResolvedByAnAbandonedTurnToo(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.repo.OpenTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", Now: t0,
	}))
	require.NoError(t, f.repo.Interrupt(f.ctx, chat, "i1", "permission", "Bash", t0))
	f.wait()

	require.NoError(t, f.repo.Abandon(f.ctx, chat, t0.Add(time.Second)))

	after, err := f.repo.Interruptions(f.ctx, chat)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.NotNil(t, after[0].ResolvedAt)
}

func TestPaging_WalksForwardAndBackwardWithoutOverlap(t *testing.T) {
	f := newFixture(t)
	for i := range 10 {
		f.turn(t, fmt.Sprintf("t%02d", i), domain.TurnRoleUser, fmt.Sprintf("m%02d", i), t0)
	}
	all, err := f.repo.Turns(f.ctx, chat, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, all, 10)

	forward, err := f.repo.Turns(f.ctx, chat, all[2].Seq, 0, 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"m03", "m04", "m05"}, textsOf(forward))

	// Backward reads the NEWEST rows below a cursor, which is why it must not read
	// the whole history to discard most of it.
	backward, err := f.repo.TurnsBefore(f.ctx, chat, all[5].Seq, 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"m02", "m03", "m04"}, textsOf(backward))

	newest, err := f.repo.TurnsBefore(f.ctx, chat, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"m08", "m09"}, textsOf(newest))
}

func TestResumeQueries_AnswerWhenAProviderLastSpoke(t *testing.T) {
	f := newFixture(t)
	f.turn(t, "t1", domain.TurnRoleUser, "hello", t0)
	require.NoError(t, f.repo.AppendTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t2", Role: domain.TurnRoleAssistant,
		ProviderID: "codex", SessionID: "codex-1", Text: "hi", Now: t0.Add(time.Minute),
	}))

	at, found, err := f.repo.LastTurnAt(f.ctx, chat, "codex")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, t0.Add(time.Minute).UTC(), at.UTC())

	_, found, err = f.repo.LastTurnAt(f.ctx, chat, "gemini")
	require.NoError(t, err)
	assert.False(t, found, "a provider that never spoke has nothing to resume")

	_, found, err = f.repo.LastTurnForSession(f.ctx, chat, "codex", "codex-1")
	require.NoError(t, err)
	assert.True(t, found)

	_, found, err = f.repo.LastTurnForSession(f.ctx, chat, "codex", "codex-9")
	require.NoError(t, err)
	assert.False(t, found)

	has, err := f.repo.HasTurnAtOrAfter(f.ctx, chat, "codex", t0)
	require.NoError(t, err)
	assert.True(t, has)

	has, err = f.repo.HasTurnAtOrAfter(f.ctx, chat, "codex", t0.Add(time.Hour))
	require.NoError(t, err)
	assert.False(t, has)
}

func TestTurnsSince_IsTheHandoffGap(t *testing.T) {
	f := newFixture(t)
	f.turn(t, "t1", domain.TurnRoleUser, "before", t0)
	f.turn(t, "t2", domain.TurnRoleUser, "after", t0.Add(time.Hour))

	gap, err := f.repo.TurnsSince(f.ctx, chat, t0.Add(30*time.Minute))

	require.NoError(t, err)
	assert.Equal(t, []string{"after"}, textsOf(gap))
}

func TestCountTurns_ReportsTheWholeConversation(t *testing.T) {
	f := newFixture(t)
	f.turn(t, "t1", domain.TurnRoleUser, "a", t0)
	f.turn(t, "t2", domain.TurnRoleUser, "b", t0)

	n, err := f.repo.CountTurns(f.ctx, chat)

	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

// The cross-agent surface: what are the other agents in this workspace touching.
func TestRecentToolCalls_SpansChatsNewestFirst(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: chat, ToolID: "a", Name: "Read", Target: "a.go", Now: t0,
	}))
	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: "chat-2", ToolID: "b", Name: "Edit", Target: "b.go", Now: t0.Add(time.Minute),
	}))
	f.wait()

	got, err := f.repo.RecentToolCalls(f.ctx, []string{chat, "chat-2"}, t0.Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "b.go", got[0].Target, "newest first")

	none, err := f.repo.RecentToolCalls(f.ctx, nil, t0, 10)
	require.NoError(t, err)
	assert.Empty(t, none)

	windowed, err := f.repo.RecentToolCalls(f.ctx, []string{chat, "chat-2"}, t0.Add(30*time.Second), 10)
	require.NoError(t, err)
	assert.Len(t, windowed, 1)
}

func TestPayload_MissingRefIsNotFound(t *testing.T) {
	f := newFixture(t)

	_, err := f.repo.Payload(f.ctx, "sha256:"+strings.Repeat("a", 64))

	assert.ErrorIs(t, err, agentactivity.ErrNotFound)
}

// A hard delete must not leave the conversation readable.
func TestForget_DropsTheRecordAndItsRows(t *testing.T) {
	f := newFixture(t)
	f.turn(t, "t1", domain.TurnRoleUser, "secret", t0)
	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: chat, ToolID: "tool-1", Name: "Read", Now: t0,
	}))
	require.NoError(t, f.repo.StartSubagent(f.ctx, chat, "a1", "explore", t0))
	require.NoError(t, f.repo.Interrupt(f.ctx, chat, "i1", "permission", "Bash", t0))
	f.wait()

	require.NoError(t, f.repo.Forget(f.ctx, chat))
	f.wait()

	turns, err := f.repo.Turns(f.ctx, chat, 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, turns)
	calls, err := f.repo.ToolCalls(f.ctx, chat, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, calls)
	subs, err := f.repo.Subagents(f.ctx, chat)
	require.NoError(t, err)
	assert.Empty(t, subs)
	ints, err := f.repo.Interruptions(f.ctx, chat)
	require.NoError(t, err)
	assert.Empty(t, ints)
}

func TestValidation_IsSurfacedAndNeverRetried(t *testing.T) {
	f := newFixture(t)

	err := f.repo.AppendTurn(f.ctx, agentactivity.TurnInput{ChatID: chat, TurnID: "t", Role: "narrator"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "role")
}

// G5. The aggregate holds OPEN state only, so its size is independent of how long
// a chat runs — which is what keeps snapshot writes and cold loads flat over a
// conversation's whole life.
func TestAggregateState_StaysBoundedOverALongConversation(t *testing.T) {
	f := newFixture(t)

	const turns = 200
	for i := range turns {
		id := fmt.Sprintf("t%03d", i)
		require.NoError(t, f.repo.OpenTurn(f.ctx, agentactivity.TurnInput{
			ChatID: chat, TurnID: id, ProviderID: "claude", Now: t0,
		}))
		for j := range 6 {
			tool := fmt.Sprintf("%s-tool-%d", id, j)
			require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
				ChatID: chat, ToolID: tool, Name: "Read", Target: "big.go",
				Request: []byte(strings.Repeat("payload ", 512)), Now: t0,
			}))
			require.NoError(t, f.repo.CompleteTool(f.ctx, agentactivity.ToolResultInput{
				ChatID: chat, ToolID: tool, Result: []byte(strings.Repeat("result ", 512)),
				Status: domain.ToolStatusOK, Now: t0,
			}))
		}
		require.NoError(t, f.repo.CloseTurn(f.ctx, agentactivity.TurnInput{
			ChatID: chat, TurnID: id, Text: "reply " + id, Now: t0,
		}))
	}
	f.wait()

	state, err := f.ax.Get(f.ctx, chat)
	require.NoError(t, err)

	encoded, err := json.Marshal(state)
	require.NoError(t, err)
	assert.Less(t, len(encoded), 8<<10,
		"a %d-turn conversation must not grow the aggregate: %d bytes", turns, len(encoded))
	assert.Zero(t, state.OpenCount())
	assert.Nil(t, state.Turn)

	// And the record itself is complete.
	count, err := f.repo.CountTurns(f.ctx, chat)
	require.NoError(t, err)
	assert.Equal(t, int64(turns), count)
	calls, err := f.repo.ToolCalls(f.ctx, chat, 0, 0)
	require.NoError(t, err)
	assert.Len(t, calls, turns*6)
}

// G6/G7. The event log is the durable record, so a lost read model is rebuilt by
// replaying it — and because every write is an upsert keyed by the item's own id,
// replaying an already-projected event rewrites identical values.
func TestReadModel_IsRebuiltByReplayAndTheRebuildIsIdempotent(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.repo.OpenTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: t0,
	}))
	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: chat, ToolID: "tool-1", Name: "Edit", Target: "a.go", Now: t0,
	}))
	require.NoError(t, f.repo.CompleteTool(f.ctx, agentactivity.ToolResultInput{
		ChatID: chat, ToolID: "tool-1", Status: domain.ToolStatusOK, DurationMS: 7, Now: t0,
	}))
	require.NoError(t, f.repo.StartSubagent(f.ctx, chat, "a1", "explore", t0))
	require.NoError(t, f.repo.Interrupt(f.ctx, chat, "i1", "compaction", "auto", t0))
	require.NoError(t, f.repo.OpenChoice(f.ctx, agentactivity.ChoiceInput{
		ChatID: chat, ChoiceID: "c1", Kind: domain.ChoiceKindPermission,
		PromptID: "p1", ToolName: "Edit",
		Options: []domain.ActivityChoiceOption{
			{ID: "allow", Kind: domain.ChoiceOptionAllow, Label: "Allow"},
		},
		Now: t0,
	}))
	require.NoError(t, f.repo.CloseTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", Text: "done", Now: t0.Add(time.Second),
	}))
	f.wait()

	before := snapshot(t, f)
	require.NotEmpty(t, before.turns)
	require.NotEmpty(t, before.calls)
	require.NotEmpty(t, before.choices)

	// A fresh read model over the SAME event log: what a lost state directory
	// looks like from the repository's point of view.
	rebuiltDB, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	rebuilt, err := agentactivity.NewEventSourced(f.ax, f.es, rebuiltDB, f.dir)
	require.NoError(t, err)

	after := snapshot(t, fixture{repo: rebuilt, ctx: f.ctx})
	assert.Equal(t, before, after, "a replayed model must reproduce the live one row for row")

	// Replaying a second time must change nothing.
	again, err := agentactivity.NewEventSourced(f.ax, f.es, rebuiltDB, f.dir)
	require.NoError(t, err)
	assert.Equal(t, before, snapshot(t, fixture{repo: again, ctx: f.ctx}),
		"the projection is idempotent, so a repeated rebuild cannot double-count")
}

type recordSnapshot struct {
	turns   []domain.ActivityTurn
	calls   []domain.ActivityToolCall
	subs    []domain.ActivitySubagent
	ints    []domain.ActivityInterruption
	choices []domain.ActivityChoice
}

func snapshot(t *testing.T, f fixture) recordSnapshot {
	t.Helper()
	turns, err := f.repo.Turns(f.ctx, chat, 0, 0, 0)
	require.NoError(t, err)
	calls, err := f.repo.ToolCalls(f.ctx, chat, 0, 0)
	require.NoError(t, err)
	subs, err := f.repo.Subagents(f.ctx, chat)
	require.NoError(t, err)
	ints, err := f.repo.Interruptions(f.ctx, chat)
	require.NoError(t, err)
	choices, err := f.repo.Choices(f.ctx, chat)
	require.NoError(t, err)
	return recordSnapshot{turns: turns, calls: calls, subs: subs, ints: ints, choices: choices}
}

func textsOf(turns []domain.ActivityTurn) []string {
	out := make([]string, 0, len(turns))
	for _, t := range turns {
		out = append(out, t.Text)
	}
	return out
}

func TestNewEventSourced_ReportsAnUnusableReadModelOrContentRoot(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentActivity]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	closed, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sql, err := closed.DB()
	require.NoError(t, err)
	require.NoError(t, sql.Close())
	_, err = agentactivity.NewEventSourced(ax, es, closed, t.TempDir())
	assert.Error(t, err, "an unusable read model must fail construction, not first write")

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	_, err = agentactivity.NewEventSourced(ax, es, db, "")
	assert.Error(t, err, "an empty content root is not a root")
}

// A payload that cannot be stored must not cost the record of the call itself:
// the tool DID run, and showing it with no arguments beats not showing it.
func TestInvokeTool_SurvivesAnUnwritableContentStore(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.Chmod(f.dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(f.dir, 0o700) })

	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: chat, ToolID: "tool-1", Name: "Bash",
		Request: []byte("arguments that cannot be stored"), Now: t0,
	}))
	require.NoError(t, f.repo.CompleteTool(f.ctx, agentactivity.ToolResultInput{
		ChatID: chat, ToolID: "tool-1", Result: []byte("result"), Now: t0,
	}))
	f.wait()

	calls, err := f.repo.ToolCalls(f.ctx, chat, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, "Bash", calls[0].Name)
	assert.Empty(t, calls[0].RequestRef)
	assert.Empty(t, calls[0].ResultRef)
}

// A lost read model is repaired lazily on first READ, not eagerly at boot: a
// normal boot must cost no replay at all.
func TestReadModel_HealsOnFirstReadWhenTheStateDirectoryWasLost(t *testing.T) {
	f := newFixture(t)
	f.turn(t, "t1", domain.TurnRoleUser, "recorded", t0)

	fresh, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	repo, err := agentactivity.NewEventSourced(f.ax, f.es, fresh, f.dir)
	require.NoError(t, err)

	turns, err := repo.Turns(f.ctx, chat, 0, 0, 0)

	require.NoError(t, err)
	require.Len(t, turns, 1)
	assert.Equal(t, "recorded", turns[0].Text)
}

// Verified live against claude 2.1.233 on 2026-08-17: a Notification saying
// "Claude is waiting for your input" arrives a MINUTE after the turn ended. Held
// open it renders a permanent "the agent needs your attention" banner over an
// agent that is idle and fine — and nothing ever resolves it, because neither
// provider has an event that ends a notification.
func TestRegression_AnInterruptionOutsideATurnIsAMomentNotABlockingState(t *testing.T) {
	f := newFixture(t)
	// A completed turn, then an interruption a minute later.
	require.NoError(t, f.repo.OpenTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: t0,
	}))
	require.NoError(t, f.repo.CloseTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", Text: "done", Now: t0.Add(time.Second),
	}))

	require.NoError(t, f.repo.Interrupt(
		f.ctx, chat, "i1", "notification", "Claude is waiting for your input", t0.Add(time.Minute)))
	f.wait()

	got, err := f.repo.Interruptions(f.ctx, chat)
	require.NoError(t, err)
	require.Len(t, got, 1, "it is still RECORDED — it happened")
	assert.NotNil(t, got[0].ResolvedAt, "but it never reads as the agent being blocked")
}

// The distinguishing fact is that the agent stopped MID-TURN, with a turn still
// open to be blocked in.
func TestInterrupt_MidTurnIsABlockingStateUntilTheTurnEnds(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.repo.OpenTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: t0,
	}))

	require.NoError(t, f.repo.Interrupt(f.ctx, chat, "i1", "permission", "Bash", t0))
	f.wait()

	blocked, err := f.repo.Interruptions(f.ctx, chat)
	require.NoError(t, err)
	require.Len(t, blocked, 1)
	assert.Nil(t, blocked[0].ResolvedAt)
	assert.Equal(t, "t1", blocked[0].TurnID, "and it belongs to the turn it blocked")
}

// An interruption outside a turn must not conjure a turn either: a notification
// about an idle agent is not the start of a reply.
func TestInterrupt_OutsideATurnDoesNotOpenOne(t *testing.T) {
	f := newFixture(t)

	require.NoError(t, f.repo.Interrupt(f.ctx, chat, "i1", "notification", "idle", t0))
	f.wait()

	turns, err := f.repo.Turns(f.ctx, chat, 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, turns)
}
