package projections_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store/internal/projections"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store/internal/storage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var now = time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)

func newProjector(t *testing.T) (*projections.Projector, *storage.Store) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := storage.New(db)
	require.NoError(t, err)
	return projections.New(st), st
}

// An event with no delta is ordinary: the reconcile emits one when there was
// nothing open, and recording that it ran is the point.
func TestApply_AnEventWithNoDeltaIsANoOp(t *testing.T) {
	p, st := newProjector(t)

	require.NoError(t, p.Apply(context.Background(), domain.AgentActivity{ChatID: "c1"}))

	empty, err := st.Empty(context.Background())
	require.NoError(t, err)
	assert.True(t, empty)
}

// A delta whose payload is missing, or whose kind nothing handles, must not
// panic: the projection runs on every event and cannot be the thing that breaks.
func TestApply_TolerossMalformedDeltas(t *testing.T) {
	p, _ := newProjector(t)
	ctx := context.Background()

	testCases := []domain.ActivityDelta{
		{Kind: domain.DeltaTurn},
		{Kind: domain.DeltaTool},
		{Kind: domain.DeltaSubagent},
		{Kind: domain.DeltaInterruption},
		{Kind: "something-new"},
	}
	for _, delta := range testCases {
		d := delta
		assert.NoError(t, p.Apply(ctx, domain.AgentActivity{ChatID: "c1", Last: &d}), d.Kind)
	}
}

// A reply is written when it CLOSES. An open turn has no text, and a blank row in
// the conversation reads as the agent having said nothing.
func TestApply_AnOpenTurnIsNotYetARow(t *testing.T) {
	p, st := newProjector(t)
	ctx := context.Background()

	require.NoError(t, p.Apply(ctx, domain.AgentActivity{ChatID: "c1", Last: &domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaTurn,
		Turn: &domain.ActivityTurn{ID: "open", ChatID: "c1", Role: domain.TurnRoleAssistant},
	}}))

	turns, err := st.Turns(ctx, "c1", 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, turns)
}

func TestApply_RepointsActivityFromTheSupersededTurn(t *testing.T) {
	p, st := newProjector(t)
	ctx := context.Background()

	require.NoError(t, st.SaveToolCall(ctx, domain.ActivityToolCall{
		ID: "tool-1", ChatID: "c1", TurnID: "open", Name: "Bash",
		Status: domain.ToolStatusOK, StartedAt: now,
	}))
	require.NoError(t, st.SaveSubagent(ctx, domain.ActivitySubagent{
		ID: "a1", ChatID: "c1", TurnID: "open", StartedAt: now,
	}))
	require.NoError(t, st.SaveInterruption(ctx, domain.ActivityInterruption{
		ID: "i1", ChatID: "c1", TurnID: "open", Kind: "permission", At: now,
	}))

	ended := now.Add(time.Second)
	require.NoError(t, p.Apply(ctx, domain.AgentActivity{ChatID: "c1", Last: &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTurn, SupersededTurnID: "open",
		Turn: &domain.ActivityTurn{
			ID: "delivery-1", ChatID: "c1", Role: domain.TurnRoleAssistant,
			Text: "done", StartedAt: now, EndedAt: &ended,
		},
	}}))

	calls, err := st.ToolCalls(ctx, "c1", 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, "delivery-1", calls[0].TurnID)

	subs, err := st.Subagents(ctx, "c1")
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "delivery-1", subs[0].TurnID)

	ints, err := st.Interruptions(ctx, "c1")
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, "delivery-1", ints[0].TurnID)
}

func TestApply_DoesNotRepointWhenTheIDsAlreadyMatch(t *testing.T) {
	p, st := newProjector(t)
	ctx := context.Background()
	require.NoError(t, st.SaveToolCall(ctx, domain.ActivityToolCall{
		ID: "tool-1", ChatID: "c1", TurnID: "same", Status: domain.ToolStatusOK, StartedAt: now,
	}))

	require.NoError(t, p.Apply(ctx, domain.AgentActivity{ChatID: "c1", Last: &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTurn, SupersededTurnID: "same",
		Turn: &domain.ActivityTurn{ID: "same", ChatID: "c1", Text: "done", StartedAt: now},
	}}))

	calls, err := st.ToolCalls(ctx, "c1", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, "same", calls[0].TurnID)
}

func TestForget_RemovesEverythingBelongingToAChat(t *testing.T) {
	p, st := newProjector(t)
	ctx := context.Background()
	require.NoError(t, st.SaveTurn(ctx, domain.ActivityTurn{ID: "t", ChatID: "c1", Text: "x"}))
	require.NoError(t, st.SaveTurn(ctx, domain.ActivityTurn{ID: "t", ChatID: "c2", Text: "y"}))

	require.NoError(t, p.Forget(ctx, "c1"))

	gone, err := st.Turns(ctx, "c1", 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, gone)

	kept, err := st.Turns(ctx, "c2", 0, 0, 0)
	require.NoError(t, err)
	assert.Len(t, kept, 1)
}

// A closed database is what a projection meets during shutdown, and every write
// path must report rather than panic.
func TestApply_ReportsAStorageFailureRatherThanPanicking(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := storage.New(db)
	require.NoError(t, err)
	p := projections.New(st)
	sql, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sql.Close())

	ctx := context.Background()
	closed := now
	deltas := []domain.ActivityDelta{
		{
			Phase: domain.DeltaClose, Kind: domain.DeltaTurn,
			Turn: &domain.ActivityTurn{ID: "t", ChatID: "c1", EndedAt: &closed},
		},
		{
			Phase: domain.DeltaClose, Kind: domain.DeltaTool,
			Tool: &domain.ActivityToolCall{ID: "x", ChatID: "c1"},
		},
		{
			Phase: domain.DeltaClose, Kind: domain.DeltaSubagent,
			Subagent: &domain.ActivitySubagent{ID: "a", ChatID: "c1"},
		},
		{
			Phase: domain.DeltaClose, Kind: domain.DeltaInterruption,
			Interruption: &domain.ActivityInterruption{ID: "i", ChatID: "c1"},
		},
	}
	for _, delta := range deltas {
		d := delta
		assert.Error(t, p.Apply(ctx, domain.AgentActivity{ChatID: "c1", Last: &d}), d.Kind)
	}
	assert.Error(t, p.Forget(ctx, "c1"))
}

func TestStorage_New_ReportsAMigrationFailure(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sql.Close())

	_, err = storage.New(db)

	assert.Error(t, err)
}

func TestStorage_QueriesReportFailureOnAClosedDatabase(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := storage.New(db)
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sql.Close())

	ctx := context.Background()
	_, err = st.Turns(ctx, "c1", 0, 0, 10)
	assert.Error(t, err)
	_, err = st.TurnsBefore(ctx, "c1", 0, 10)
	assert.Error(t, err)
	_, err = st.TurnsSince(ctx, "c1", now)
	assert.Error(t, err)
	_, err = st.CountTurns(ctx, "c1")
	assert.Error(t, err)
	_, err = st.ToolCalls(ctx, "c1", 0, 10)
	assert.Error(t, err)
	_, err = st.Subagents(ctx, "c1")
	assert.Error(t, err)
	_, err = st.Interruptions(ctx, "c1")
	assert.Error(t, err)
	_, err = st.RecentToolCalls(ctx, []string{"c1"}, now, 10)
	assert.Error(t, err)
	_, err = st.HasTurnAtOrAfter(ctx, "c1", "claude", now)
	assert.Error(t, err)
	_, err = st.Empty(ctx)
	assert.Error(t, err)
	assert.Error(t, st.RepointActivity(ctx, "c1", "a", "b"))
	assert.Error(t, st.AbandonRunningTools(ctx, "c1", nil))
}

// A provider that never spoke has nothing to resume, and that absence is an
// answer rather than a failure.
func TestStorage_ResumeQueriesTreatAbsenceAsAnAnswer(t *testing.T) {
	_, st := newProjector(t)
	ctx := context.Background()

	_, found, err := st.LastTurnAt(ctx, "c1", "claude")
	require.NoError(t, err)
	assert.False(t, found)

	_, found, err = st.LastTurnForSession(ctx, "c1", "claude", "s1")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestStorage_ToolCallsPageForward(t *testing.T) {
	_, st := newProjector(t)
	ctx := context.Background()
	for i, id := range []string{"a", "b", "c"} {
		require.NoError(t, st.SaveToolCall(ctx, domain.ActivityToolCall{
			ID: id, ChatID: "c1", Seq: int64(i + 1), Name: id, StartedAt: now,
		}))
	}

	after, err := st.ToolCalls(ctx, "c1", 1, 0)
	require.NoError(t, err)
	require.Len(t, after, 2)
	assert.Equal(t, "b", after[0].Name)

	limited, err := st.ToolCalls(ctx, "c1", 0, 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1)
}
