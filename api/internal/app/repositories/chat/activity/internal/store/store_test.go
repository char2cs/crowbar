package store_test

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
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const chat = "chat-1"

var now = time.Date(2026, 8, 17, 7, 0, 0, 0, time.UTC)

func newAsynx(t *testing.T, es asynxModels.Store) asynx.Asynx[domain.ChatActivity] {
	t.Helper()
	ax, err := asynx.New[domain.ChatActivity]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

func TestNew_ReportsAnUnusableReadModel(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sql.Close())

	_, err = store.New(db, t.TempDir(), newAsynx(t, es), es)

	assert.Error(t, err)
}

func TestNew_ReportsAnUnusableContentRoot(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	_, err = store.New(db, "", newAsynx(t, es), es)

	assert.Error(t, err)
}

func TestOnEvent_SwallowsAStorageFailure(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	ax := newAsynx(t, es)
	_, err = store.New(db, t.TempDir(), ax, es)
	require.NoError(t, err)

	sql, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sql.Close())

	_, sendErr := ax.SendWait(context.Background(), commands.AppendTurn{
		ChatID: chat, TurnID: "t1", Role: domain.TurnRoleUser, Text: "hi", Now: now,
	})

	assert.NoError(t, sendErr, "the write succeeds; only its projection failed")
}

func TestOnForget_SwallowsAStorageFailure(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	ax := newAsynx(t, es)
	_, err = store.New(db, t.TempDir(), ax, es)
	require.NoError(t, err)
	_, err = ax.SendWait(context.Background(), commands.AppendTurn{
		ChatID: chat, TurnID: "t1", Role: domain.TurnRoleUser, Text: "hi", Now: now,
	})
	require.NoError(t, err)

	sql, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sql.Close())

	assert.NoError(t, ax.Forget(context.Background(), chat))
}

func TestHeal_RunsAtMostOnceAndOnlyForAnEmptyModel(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax := newAsynx(t, es)

	live, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	written, err := store.New(live, t.TempDir(), ax, es)
	require.NoError(t, err)
	_, err = ax.SendWait(context.Background(), commands.AppendTurn{
		ChatID: chat, TurnID: "t1", Role: domain.TurnRoleUser, Text: "recorded", Now: now,
	})
	require.NoError(t, err)

	turns, err := written.Turns(context.Background(), chat, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, turns, 1)

	fresh, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	healed, err := store.New(fresh, t.TempDir(), ax, es)
	require.NoError(t, err)

	got, err := healed.Turns(context.Background(), chat, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "recorded", got[0].Text)

	again, err := healed.Turns(context.Background(), "chat-with-nothing", 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, again)
}

func TestHeal_IsSkippedWhenTheEventStoreCannotEnumerate(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	s, err := store.New(db, t.TempDir(), newAsynx(t, es), noLister{es})
	require.NoError(t, err)

	turns, err := s.Turns(context.Background(), chat, 0, 0, 0)

	require.NoError(t, err)
	assert.Empty(t, turns)
}

type noLister struct{ inner asynxModels.Store }

func (n noLister) Append(ctx context.Context, id string, version int64, data []byte) error {
	return n.inner.Append(ctx, id, version, data)
}

func (n noLister) ReadFrom(ctx context.Context, id string, from int64) ([][]byte, error) {
	return n.inner.ReadFrom(ctx, id, from)
}

func (n noLister) ReadRange(ctx context.Context, id string, from, count int64) ([][]byte, error) {
	return n.inner.ReadRange(ctx, id, from, count)
}

func (n noLister) Delete(ctx context.Context, id string) error {
	return n.inner.Delete(ctx, id)
}
