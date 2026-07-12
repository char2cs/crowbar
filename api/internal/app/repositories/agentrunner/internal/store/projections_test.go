package store

// Internal tests for the projection internals that the external store_test
// cannot reach: the event-name parser's fallback, and the fold's error branches
// (which only log, so their effect is observable as "does not panic, does not
// corrupt the model").

import (
	"context"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestEventKind_ParsesAndFallsBack(t *testing.T) {
	assert.Equal(t, "started", eventKind("agentrunner.started.r1"))
	assert.Equal(t, "session_bound", eventKind("agentrunner.session_bound.r1"))
	assert.Equal(t, "moved", eventKind("agentrunner.moved.r1"))
	assert.Equal(t, "exited", eventKind("agentrunner.exited.r1"))

	// A name that doesn't fit "agentrunner.<kind>.<id>" still yields a frame
	// rather than being silently dropped.
	assert.Equal(t, "odd", eventKind("agentrunner.odd"))
	assert.Equal(t, "unprefixed", eventKind("unprefixed"))
}

func newProjectorDB(
	t *testing.T,
) (*projector, func()) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runnerRow{}, &conversationRow{}))
	closeDB := func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}
	return &projector{db: db}, closeDB
}

func evt(
	r domain.AgentRunner,
) asynxModels.Event[domain.AgentRunner] {
	return asynxModels.Event[domain.AgentRunner]{AggregateID: r.ID, Aggregate: r}
}

// A write failure in any of the three fold branches is logged, not fatal — the
// projection must never take the daemon down with it.
func TestProjector_WriteFailuresAreLoggedNotFatal(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	closeDB()

	live := domain.AgentRunner{
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

// A runner with no conversation yet (spawned, provider has not announced) gets a
// live row but appends NO history — history only records conversations that
// actually existed.
func TestProjector_UnboundRunnerAppendsNoConversation(t *testing.T) {
	p, closeDB := newProjectorDB(t)
	defer closeDB()

	p.onEvent(context.Background(), evt(domain.AgentRunner{
		ID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", CurrentChatID: "c1", StartedAt: time.Unix(1, 0),
	}))

	var runners int64
	require.NoError(t, p.db.Model(&runnerRow{}).Count(&runners).Error)
	assert.Equal(t, int64(1), runners)

	var convs int64
	require.NoError(t, p.db.Model(&conversationRow{}).Count(&convs).Error)
	assert.Equal(t, int64(0), convs, "no conversation was ever announced")
}
