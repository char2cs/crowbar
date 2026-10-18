package store

// Internal tests for the projection internals that the external store_test
// cannot reach: the event-name parser's fallback, and the fold's error branches
// (which only log, so their effect is observable as "does not panic, does not
// corrupt the model").

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

func TestEventKind_ParsesAndFallsBack(t *testing.T) {
	assert.Equal(t, "started", eventKind("runner.started.r1"))
	assert.Equal(t, "session_bound", eventKind("runner.session_bound.r1"))
	assert.Equal(t, "exited", eventKind("runner.exited.r1"))

	// A name that doesn't fit "runner.<kind>.<id>" still yields a frame
	// rather than being silently dropped.
	assert.Equal(t, "odd", eventKind("runner.odd"))
	assert.Equal(t, "unprefixed", eventKind("unprefixed"))
}

func newProjectorDB(
	t *testing.T,
) (*projector, func()) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runnerRow{}, &placementRow{}))
	closeDB := func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}
	return &projector{db: db}, closeDB
}

func evt(
	r agents.Runner,
) asynxModels.Event[agents.Runner] {
	return asynxModels.Event[agents.Runner]{AggregateID: r.ID, Aggregate: r}
}

// A write failure in any of the fold branches is logged, not fatal — the
// projection must never take the daemon down with it.
func TestProjector_WriteFailuresAreLoggedNotFatal(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	closeDB()

	live := agents.Runner{
		ID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", CurrentChatID: "c1",
		StartedAt: time.Unix(1, 0),
	}
	assert.NotPanics(t, func() { p.onEvent(context.Background(), evt(live)) })

	exited := live
	at := time.Unix(2, 0)
	exited.ExitedAt = &at
	assert.NotPanics(t, func() { p.onEvent(context.Background(), evt(exited)) })
}

// Every runner leaves a durable record naming its provider on its chat, alongside
// its live row.
func TestProjector_APlacementIsRecordedForEveryRunner(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	defer closeDB()

	p.onEvent(context.Background(), evt(agents.Runner{
		ID: "r1", WorkspaceID: "w1", ProviderID: "quietvendor",
		TerminalSession: "pty1", CurrentChatID: "c1", StartedAt: time.Unix(1, 0),
	}))

	var runners int64
	require.NoError(t, p.db.Model(&runnerRow{}).Count(&runners).Error)
	assert.Equal(t, int64(1), runners)

	var rows []placementRow
	require.NoError(t, p.db.Where("chat_id = ?", "c1").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "quietvendor", rows[0].ProviderID)
	assert.Equal(t, time.Unix(1, 0).UTC(), rows[0].LastPlacedAt.UTC())
}

// TestProjector_APlacementTimestampNeverMovesBackwards pins appendPlacement's
// MAX. Two runners of one provider can land on a chat concurrently, and their
// events are projected in arrival order, not timestamp order — a stale one
// dragging the provider's arrival backwards could hand the chat to a DIFFERENT
// provider whose own arrival sits between them.
func TestProjector_APlacementTimestampNeverMovesBackwards(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	defer closeDB()

	late := agents.Runner{
		ID: "r2", WorkspaceID: "w1", ProviderID: "vendor-a",
		CurrentChatID: "c1", StartedAt: time.Unix(9, 0),
	}
	early := agents.Runner{
		ID: "r1", WorkspaceID: "w1", ProviderID: "vendor-a",
		CurrentChatID: "c1", StartedAt: time.Unix(2, 0),
	}
	p.onEvent(context.Background(), evt(late))
	p.onEvent(context.Background(), evt(early))

	var rows []placementRow
	require.NoError(t, p.db.Where("chat_id = ?", "c1").Find(&rows).Error)
	require.Len(t, rows, 1, "one row per (chat, provider), however many runners")
	assert.Equal(t, time.Unix(9, 0).UTC(), rows[0].LastPlacedAt.UTC())
}

// A DISPLACED runner is pointed at no chat, and must record no placement: a
// placement on "" would be a claim about a chat that does not exist.
func TestProjector_ADisplacedRunnerRecordsNoPlacement(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	defer closeDB()

	p.onEvent(context.Background(), evt(agents.Runner{
		ID: "r1", WorkspaceID: "w1", ProviderID: "vendor-a",
		CurrentChatID: "", StartedAt: time.Unix(1, 0),
	}))

	var placements int64
	require.NoError(t, p.db.Model(&placementRow{}).Count(&placements).Error)
	assert.Equal(t, int64(0), placements)
}
