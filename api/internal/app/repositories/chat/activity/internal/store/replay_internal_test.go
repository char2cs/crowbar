package store

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestOnForget_FallsBackToAggregateChatIDWhenAggregateIDEmpty pins onForget's
// defensive fallback. In the ordinary ax.Forget(ctx, id) path the tombstone
// event's AggregateID is always the forgotten id (asynx's ForgetCommand sets
// it directly), so this branch is never exercised through the public API —
// it exists purely to keep onForget correct if that ever stops holding. Direct,
// white-box invocation is the only way to exercise it: a synthetic event with a
// blank AggregateID but a populated Aggregate.ChatID must still resolve to, and
// forget, the RIGHT chat rather than silently forgetting "".
func TestOnForget_FallsBackToAggregateChatIDWhenAggregateIDEmpty(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ChatActivity]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	s, err := New(db, t.TempDir(), ax, es)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ax.SendWait(ctx, commands.AppendTurn{
		ChatID: "c1", TurnID: "t1", Role: domain.TurnRoleUser, Text: "hi",
		Now: time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	turns, err := s.Turns(ctx, "c1", 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, turns, 1, "precondition: c1 has a projected turn before the forget")

	// A tombstone event whose AggregateID is blank — onForget must still
	// resolve "c1" from evt.Aggregate.ChatID, not silently forget "".
	s.onForget(ctx, asynxModels.Event[domain.ChatActivity]{
		AggregateID: "",
		Aggregate:   domain.ChatActivity{ChatID: "c1"},
	})

	after, err := s.Turns(ctx, "c1", 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, after, "onForget must fall back to Aggregate.ChatID and forget the right chat")
}

// TestRebuild_LogsProjectionApplyFailure_DoesNotAbortReplay pins
// foldReplayed's own contract: asynx.Replay's ProjectionHandler signature has
// no error return, so a projector.Apply failure mid-fold can only ever be
// logged, never propagated back through Replay — and rebuild (which treats
// a genuine Replay error as fatal, see store.go) must not be tricked into
// thinking the replay itself failed just because the WRITE inside it did.
func TestRebuild_LogsProjectionApplyFailure_DoesNotAbortReplay(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ChatActivity]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	s, err := New(db, t.TempDir(), ax, es)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ax.SendWait(ctx, commands.AppendTurn{
		ChatID: "c1", TurnID: "t1", Role: domain.TurnRoleUser, Text: "hi",
		Now: time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	// Break the storage the projector writes through, but keep s.rebuild
	// itself callable: foldReplayed's projector.Apply call now fails on every
	// row it tries to persist.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = s.rebuild(ctx)
	require.NoError(t, err,
		"a projection write failure during replay must be logged, not surfaced as a rebuild error")
}
