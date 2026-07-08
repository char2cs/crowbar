package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestNew_RegistersStoreProjection proves the reworked store.New wires the
// save-only projection onto the singleton axWorkspace and does NO eager reconcile
// (the List reflects only what the projection saved). SendWait is used only to
// make the projection deterministic in-test.
func TestNew_RegistersStoreProjection(t *testing.T) {
	ctx := context.Background()

	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, ax)
	require.NoError(t, err)

	// Empty before any event — no reconcile ran.
	all, err := st.List(ctx)
	require.NoError(t, err)
	require.Empty(t, all)

	_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	all, err = st.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "w1", all[0].ID)
	assert.Equal(t, "p1", all[0].ProjectID)
	assert.Equal(t, "r1", all[0].RepoID)
}
