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
	assert.Equal(t, "moved", eventKind("runner.moved.r1"))
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
	require.NoError(t, db.AutoMigrate(&runnerRow{}, &conversationRow{}, &placementRow{}, &healMarkerRow{}))
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

// A write failure in any of the three fold branches is logged, not fatal — the
// projection must never take the daemon down with it.
func TestProjector_WriteFailuresAreLoggedNotFatal(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	closeDB()

	live := agents.Runner{
		ID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", CurrentChatID: "c1", CurrentSession: "s1",
		StartedAt: time.Unix(1, 0),
	}
	assert.NotPanics(t, func() { p.onEvent(context.Background(), evt(live)) })

	exited := live
	at := time.Unix(2, 0)
	exited.ExitedAt = &at
	assert.NotPanics(t, func() { p.onEvent(context.Background(), evt(exited)) })
}

// The heal must not mistake a broken read DB for a virgin one: an unreadable
// marker is a hard error at construction, never a silent full replay.
func TestHealHistory_MarkerReadFailureSurfaces(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	closeDB()

	err := healHistory(p.db, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read heal marker")
}

// The markers are written after the heal, so a first construction that FAILS is
// retried on the next boot rather than being recorded as done — and EVERY
// projection gets its own, which is what lets a new one be folded on a read DB
// that already believes itself built.
func TestHealHistory_MarksEveryProjectionBuilt(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	defer closeDB()

	require.NoError(t, healHistory(p.db, nil, nil))

	for _, markerID := range []string{conversationsMarkerID, placementsMarkerID} {
		built, err := readModelWasBuilt(context.Background(), p.db, markerID)
		require.NoError(t, err)
		assert.True(t, built, "an empty history table now means 'nothing to say', not 'heal me': "+markerID)
	}
}

// TestRegression_HealHistory_APreExistingReadModelStillFoldsANewProjection is the
// recovery half of the resume bug. A read DB built before the placement
// projection existed carries the CONVERSATIONS marker and nothing else; under one
// shared marker it would report itself built and every chat already on disk would
// stay unresolvable forever. Per-projection markers make the placement fold
// PENDING on exactly that DB.
func TestRegression_HealHistory_APreExistingReadModelStillFoldsANewProjection(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	defer closeDB()

	require.NoError(t, markBuilt(context.Background(), p.db,
		[]historyFold{{markerID: conversationsMarkerID}}))

	pending, err := pendingFolds(context.Background(), p.db)
	require.NoError(t, err)
	require.Len(t, pending, 1, "the conversations fold is done; the placement fold is not")
	assert.Equal(t, placementsMarkerID, pending[0].markerID)
}

// TestProjector_APlacementIsRecordedForARunnerThatAnnouncedNothing is the whole
// point of the placement projection: the runner that leaves NO conversation row
// (TestProjector_UnboundRunnerAppendsNoConversation below proves it leaves none)
// still leaves a durable record naming its provider on its chat.
func TestProjector_APlacementIsRecordedForARunnerThatAnnouncedNothing(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	defer closeDB()

	p.onEvent(context.Background(), evt(agents.Runner{
		ID: "r1", WorkspaceID: "w1", ProviderID: "quietvendor",
		TerminalSession: "pty1", CurrentChatID: "c1", StartedAt: time.Unix(1, 0),
	}))

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

// A runner with no conversation yet (spawned, provider has not announced) gets a
// live row but appends NO history — history only records conversations that
// actually existed.
func TestProjector_UnboundRunnerAppendsNoConversation(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	defer closeDB()

	p.onEvent(context.Background(), evt(agents.Runner{
		ID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", CurrentChatID: "c1", StartedAt: time.Unix(1, 0),
	}))

	var runners int64
	require.NoError(t, p.db.Model(&runnerRow{}).Count(&runners).Error)
	assert.Equal(t, int64(1), runners)

	var convs int64
	require.NoError(t, p.db.Model(&conversationRow{}).Count(&convs).Error)
	assert.Equal(t, int64(0), convs, "no conversation was ever announced")

	var placements int64
	require.NoError(t, p.db.Model(&placementRow{}).Count(&placements).Error)
	assert.Equal(t, int64(1), placements,
		"but the PLACEMENT is recorded — it is what still names this chat's provider")
}
