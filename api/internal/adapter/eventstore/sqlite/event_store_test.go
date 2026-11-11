package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/char2cs/asynx/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/glebarez/go-sqlite" // registers the "sqlite" driver for the pre-v1 fixture
)

func openTemp(t *testing.T) *eventStore {
	t.Helper()
	return openAt(t, filepath.Join(t.TempDir(), "events.db"))
}

func openAt(t *testing.T, path string) *eventStore {
	t.Helper()
	s, err := NewEventStore(path)
	require.NoError(t, err)
	es, ok := s.(*eventStore)
	require.True(t, ok)
	t.Cleanup(func() { require.NoError(t, es.Close()) })
	return es
}

func cancelledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func appendAll(t *testing.T, s models.Store, streamID string, blobs ...string) {
	t.Helper()
	require.NoError(t, s.Atomically(context.Background(), func(tx models.Tx) error {
		for i, b := range blobs {
			if err := tx.Append(context.Background(), streamID, int64(i+1), []byte(b)); err != nil {
				return err
			}
		}
		return nil
	}))
}

func TestEventStore_Conformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) models.Store {
		t.Helper()
		return openTemp(t)
	})
}

func TestNewEventStore_InvalidPath_ReturnsError(t *testing.T) {
	_, err := NewEventStore("/nonexistent-dir-crowbar/db.sqlite")
	assert.Error(t, err)
}

// TestNewEventStore_ReadonlyDB_FailsToOpen: a store whose file and directory
// cannot be written refuses to open instead of serving a log it cannot append to.
func TestNewEventStore_ReadonlyDB_FailsToOpen(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission denial has no effect")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ro.sqlite")

	s, err := NewEventStore(path)
	require.NoError(t, err)
	es, ok := s.(*eventStore)
	require.True(t, ok)
	require.NoError(t, es.Close())

	require.NoError(t, os.Chmod(path, 0o444))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() {
		require.NoError(t, os.Chmod(dir, 0o755))
		require.NoError(t, os.Chmod(path, 0o644))
	})

	_, err = NewEventStore(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "readonly database")
}

// Reads run beside an open write transaction under WAL; only writes queue.
func TestEventStore_ReadsDoNotWaitForAnOpenTransaction(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	appendAll(t, s, "events:a", "e1")

	held := make(chan struct{})
	release := make(chan struct{})
	committed := make(chan error, 1)
	go func() {
		committed <- s.Atomically(ctx, func(tx models.Tx) error {
			if err := tx.Append(ctx, "events:b", 1, []byte("open")); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	blobs, err := s.ReadFrom(ctx, "events:a", 1)
	require.NoError(t, err)
	assert.Len(t, blobs, 1)
	_, err = s.ReadRange(ctx, "events:a", 1, 1)
	require.NoError(t, err)
	_, err = s.PendingDeliveries(ctx, 10)
	require.NoError(t, err)
	_, _, err = s.GetReceipt(ctx, "cmd")
	require.NoError(t, err)

	close(release)
	require.NoError(t, <-committed)
}

// A log written before v1 (events table only, gorm's DDL of the time) opens
// and reads back unchanged.
func TestEventStore_OpensADatabaseWrittenBeforeV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), "CREATE TABLE `events` (`aggregate_id` text,`version` integer,`data` blob NOT NULL,PRIMARY KEY (`aggregate_id`,`version`))")
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), "INSERT INTO events (aggregate_id, version, data) VALUES ('events:a', 1, 'e1'), ('events:a', 2, 'e2')")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s := openAt(t, path)
	blobs, err := s.ReadFrom(context.Background(), "events:a", 1)
	require.NoError(t, err)
	assert.Equal(t, [][]byte{[]byte("e1"), []byte("e2")}, blobs)
	pending, err := s.PendingDeliveries(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, pending)
	err = s.Atomically(context.Background(), func(tx models.Tx) error {
		return tx.Append(context.Background(), "events:a", 2, []byte("dup"))
	})
	assert.ErrorIs(t, err, models.ErrPipelineFailed)
}

func TestEventStore_AnAppendThatIsNotAConflictIsNotAPipelineFailure(t *testing.T) {
	s := openTemp(t)
	err := s.Atomically(context.Background(), func(tx models.Tx) error {
		return tx.Append(cancelledCtx(), "events:a", 1, []byte("x"))
	})
	require.Error(t, err)
	assert.NotErrorIs(t, err, models.ErrPipelineFailed)
}

func TestEventStore_Native_IsTheSQLTransaction(t *testing.T) {
	s := openTemp(t)
	require.NoError(t, s.Atomically(context.Background(), func(tx models.Tx) error {
		_, ok := tx.Native().(*sql.Tx)
		assert.True(t, ok)
		return nil
	}))
}

func TestEventStore_AggregateIDs_ReturnsDistinct(t *testing.T) {
	s := openTemp(t)
	appendAll(t, s, "agg-1", "a", "b")
	appendAll(t, s, "agg-2", "c")

	ids, err := s.AggregateIDs(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"agg-1", "agg-2"}, ids)
}

func TestEventStore_AggregateIDs_ContextCancelled(t *testing.T) {
	s := openTemp(t)
	_, err := s.AggregateIDs(cancelledCtx())
	assert.Error(t, err)
}

func TestEventStore_AggregateIDs_EmptyStore(t *testing.T) {
	s := openTemp(t)
	ids, err := s.AggregateIDs(context.Background())
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestEventStore_InMemoryStoreServesATransaction(t *testing.T) {
	s := openAt(t, ":memory:")
	appendAll(t, s, "events:a", "e1")
	blobs, err := s.ReadFrom(context.Background(), "events:a", 1)
	require.NoError(t, err)
	assert.Len(t, blobs, 1)
}

type thing struct {
	Name string `json:"name"`
}

// A delivery committed with its event but never handled (a crash between the
// two) is redelivered by the next Build over the reopened file, then acked.
func TestEventStore_APendingDeliverySurvivesAReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.db")
	ctx := context.Background()
	evt, err := json.Marshal(map[string]any{
		"id": "e1", "event_name": "thing.created.a", "version": 1, "schema_version": 1,
		"occurred_at": time.Unix(1, 0).UTC(),
		"patches":     []map[string]any{{"op": "replace", "path": "/name", "value": "x"}},
	})
	require.NoError(t, err)
	first, err := NewEventStore(path)
	require.NoError(t, err)
	require.NoError(t, first.Atomically(ctx, func(tx models.Tx) error {
		return errors.Join(tx.Append(ctx, "events:a", 1, evt), tx.EnqueueDelivery(ctx, "a", 1))
	}))
	closer, ok := first.(*eventStore)
	require.True(t, ok)
	require.NoError(t, closer.Close())

	s := openAt(t, path)
	got := make(chan thing, 1)
	ax, err := asynx.New[thing]().WithEventStore(s).WithSnapshotStore(asynxstore.NewSnapshots()).
		WithSubscriber(asynx.Topic("thing.*"), func(_ context.Context, e models.Event[thing]) error {
			got <- e.Aggregate
			return nil
		}).Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ax.Shutdown(context.Background())) })

	select {
	case th := <-got:
		assert.Equal(t, "x", th.Name)
	case <-time.After(10 * time.Second):
		t.Fatal("the pending delivery was never redelivered")
	}
	require.Eventually(t, func() bool {
		pending, err := s.PendingDeliveries(ctx, 10)
		return err == nil && len(pending) == 0
	}, 10*time.Second, 5*time.Millisecond, "the redelivered event must be acked")
}
