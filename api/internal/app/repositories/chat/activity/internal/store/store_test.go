package store_test

import (
	"context"
	"errors"
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
	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
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

// fakeActivityAx lets New's own ax.Subscribe / ax.OnForget calls be forced to
// fail on demand, so New's two error wraps can be pinned independently of
// each other and of storage/content construction failing first.
type fakeActivityAx struct {
	asynx.Asynx[domain.ChatActivity]
	subscribeErr error
	forgetErr    error
}

func (f *fakeActivityAx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.ChatActivity],
	_ ...asynxModels.SubscriptionOpt[domain.ChatActivity],
) (string, error) {
	if f.subscribeErr != nil {
		return "", f.subscribeErr
	}
	return "sub-id", nil
}

func (f *fakeActivityAx) OnForget(
	_ asynxModels.ForgetHandler[domain.ChatActivity],
) (string, error) {
	if f.forgetErr != nil {
		return "", f.forgetErr
	}
	return "onforget-id", nil
}

func TestNew_SubscribeError(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	_, err = store.New(db, t.TempDir(), &fakeActivityAx{subscribeErr: errors.New("bus down")}, es)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentactivity store: subscribe")
}

func TestNew_OnForgetError(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	_, err = store.New(db, t.TempDir(), &fakeActivityAx{forgetErr: errors.New("bus down")}, es)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentactivity store: on forget")
}

// erroringLister is a serialize.AggregateLister whose AggregateIDs always
// errors — standing in for an event store adapter whose enumeration call
// itself fails (as against noLister's total lack of the capability).
type erroringLister struct {
	asynxModels.Store
	err error
}

func (e erroringLister) AggregateIDs(context.Context) ([]string, error) {
	return nil, e.err
}

// TestHeal_SwallowsRebuildEnumerationError proves heal's own contract: a
// rebuild failure (here, the event store's enumeration call itself erroring)
// is logged, never propagated to the caller — an ordinary read must still
// succeed (degrading to "nothing healed yet") rather than fail outright
// because the read-model happened to be empty when it ran.
func TestHeal_SwallowsRebuildEnumerationError(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax := newAsynx(t, es)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	s, err := store.New(db, t.TempDir(), ax, erroringLister{err: errors.New("boom")})
	require.NoError(t, err)

	turns, err := s.Turns(context.Background(), chat, 0, 0, 0)
	require.NoError(t, err, "a rebuild enumeration failure must not fail an ordinary read")
	assert.Empty(t, turns)
}

// TestHeal_SwallowsAggregateReplayFailure proves the OTHER rebuild failure
// mode — enumeration succeeds, but Replaying one of the returned aggregate
// ids fails (its stored bytes were never written by a domain.ChatActivity
// command) — is likewise logged and swallowed by heal rather than failing the
// read. Unlike the sibling chat/internal/store package, THIS rebuild aborts
// the loop on the first Replay error rather than skipping past it (see
// store.go's rebuild), so this also pins that a single corrupt aggregate does
// not panic or hang an ordinary read.
func TestHeal_SwallowsAggregateReplayFailure(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax := newAsynx(t, es)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	require.NoError(t, es.Append(context.Background(), "events:poison", 1, []byte("{not valid json")))

	s, err := store.New(db, t.TempDir(), ax, es)
	require.NoError(t, err)

	turns, err := s.Turns(context.Background(), "poison", 0, 0, 0)
	require.NoError(t, err, "a single unreplayable aggregate must not fail an ordinary read")
	assert.Empty(t, turns)
}

// extraKeyLister wraps a real serialize.AggregateLister and injects one extra
// key that does not carry the "events:" prefix rebuild strips — standing in
// for a non-event row (e.g. a "snapshots:<id>" entry) the shared event log's
// raw key scan can return alongside genuine "events:<id>" rows.
type extraKeyLister struct {
	real  serialize.AggregateLister
	inner asynxModels.Store
	extra string
}

func (e extraKeyLister) Append(ctx context.Context, id string, version int64, data []byte) error {
	return e.inner.Append(ctx, id, version, data)
}

func (e extraKeyLister) ReadFrom(ctx context.Context, id string, from int64) ([][]byte, error) {
	return e.inner.ReadFrom(ctx, id, from)
}

func (e extraKeyLister) ReadRange(ctx context.Context, id string, from, count int64) ([][]byte, error) {
	return e.inner.ReadRange(ctx, id, from, count)
}

func (e extraKeyLister) Delete(ctx context.Context, id string) error {
	return e.inner.Delete(ctx, id)
}

func (e extraKeyLister) AggregateIDs(ctx context.Context) ([]string, error) {
	keys, err := e.real.AggregateIDs(ctx)
	if err != nil {
		return nil, err
	}
	return append(keys, e.extra), nil
}

// TestHeal_Rebuild_SkipsNonEventKey mirrors
// TestHeal_RunsAtMostOnceAndOnlyForAnEmptyModel's fixture but injects a bogus
// non-"events:" key alongside the real one: rebuild's CutPrefix guard must
// skip it rather than trying to Replay the raw key as an aggregate id (which
// would abort the whole rebuild on this package's abort-on-first-error
// rebuild — see TestHeal_SwallowsAggregateReplayFailure), while the real
// aggregate beside it still heals.
func TestHeal_Rebuild_SkipsNonEventKey(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax := newAsynx(t, es)

	live, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	_, err = store.New(live, t.TempDir(), ax, es)
	require.NoError(t, err)
	_, err = ax.SendWait(context.Background(), commands.AppendTurn{
		ChatID: chat, TurnID: "t1", Role: domain.TurnRoleUser, Text: "recorded", Now: now,
	})
	require.NoError(t, err)

	lister, ok := es.(serialize.AggregateLister)
	require.True(t, ok, "precondition: the real event store must support enumeration")

	fresh, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	healed, err := store.New(fresh, t.TempDir(), ax,
		extraKeyLister{real: lister, inner: es, extra: "snapshots:ghost"})
	require.NoError(t, err)

	got, err := healed.Turns(context.Background(), chat, 0, 0, 0)
	require.NoError(t, err, "the bogus non-events key must be skipped rather than failing the rebuild")
	require.Len(t, got, 1)
	assert.Equal(t, "recorded", got[0].Text)
}
