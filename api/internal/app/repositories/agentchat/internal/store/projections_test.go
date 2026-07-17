package store

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
	accmds "github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newProjected registers BOTH the save-only store projection and the hub
// broadcast projection (hub.go) on a real asynx over throwaway DBs — the
// production shape (one of each per singleton, wired together in New).
func newProjected(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.AgentChat], storage, *captureHub) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentChat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)

	h := &captureHub{}
	require.NoError(t, registerStoreProjection(st, ax))
	require.NoError(t, registerHubProjection(ax, h.push))
	return context.Background(), ax, st, h
}

func TestStoreProjection_CreateUpsertsRow(t *testing.T) {
	ctx, ax, st, _ := newProjected(t)
	_, err := ax.SendWait(ctx, accmds.Create{
		ID: "c1", WorkspaceID: "w1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	row, err := st.FindByKey(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "w1", row.WorkspaceID)
}

func TestStoreProjection_ForgetDeletesRow(t *testing.T) {
	ctx, ax, st, _ := newProjected(t)
	_, err := ax.SendWait(ctx, accmds.Create{
		ID: "c1", WorkspaceID: "w1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, ax.Forget(ctx, "c1"))

	row, err := st.FindByKey(ctx, "c1")
	require.NoError(t, err)
	assert.Nil(t, row)
}

// fakeAx is a minimal asynx stub that fails Subscribe / OnForget on demand.
type fakeAx struct {
	asynx.Asynx[domain.AgentChat]
	subscribeErr error
	forgetErr    error
	subscribeCnt int
}

func (f *fakeAx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.AgentChat],
	_ ...asynxModels.SubscriptionOpt[domain.AgentChat],
) (string, error) {
	f.subscribeCnt++
	if f.subscribeCnt == 1 {
		return "", f.subscribeErr
	}
	return "sub-id", nil
}

func (f *fakeAx) OnForget(
	_ asynxModels.ForgetHandler[domain.AgentChat],
) (string, error) {
	return "", f.forgetErr
}

func TestRegisterStoreProjection_SubscribeError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	err = registerStoreProjection(st, &fakeAx{subscribeErr: errors.New("bus down")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat store projection: subscribe")
}

func TestRegisterStoreProjection_OnForgetError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	err = registerStoreProjection(st, &fakeAx{forgetErr: errors.New("bus down")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat store projection: on forget")
}

func TestStoreProjector_OnEvent_SaveError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	p := &storeProjector{storage: &storageStore{inner: &errInner{err: errors.New("db down")}}}
	p.onEvent(ctx, asynxModels.Event[domain.AgentChat]{Aggregate: domain.AgentChat{ID: "c1"}})
}

func TestStoreProjector_OnForget_DeleteError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	p := &storeProjector{storage: &storageStore{inner: &errInner{err: errors.New("db down")}}}
	p.onForget(ctx, asynxModels.Event[domain.AgentChat]{Aggregate: domain.AgentChat{ID: "c1"}})
}

func TestNew_ProjectionsError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	ax := &fakeAx{subscribeErr: errors.New("bus down")}
	_, err = New(db, nil, ax, func(string, string, string, bool) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat store: projections")
}
