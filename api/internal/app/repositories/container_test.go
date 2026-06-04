package repositories_test

import (
	"context"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func ax[T any](t *testing.T) asynx.Asynx[T] {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	a, err := asynx.New[T]().WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })
	return a
}

func TestContainer_New_BuildsRepos(t *testing.T) {
	c := repositories.New(
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		ax[domain.AgentRun](t),
		ax[domain.ReviewThread](t),
	)
	assert.NotNil(t, c.Workspace)
	assert.NotNil(t, c.Chat)
	assert.NotNil(t, c.AgentRun)
	assert.NotNil(t, c.ReviewThread)
}

func TestContainer_RegisterHubProjections_StubNoError(t *testing.T) {
	c := repositories.New(
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		ax[domain.AgentRun](t),
		ax[domain.ReviewThread](t),
	)
	assert.NoError(t, c.RegisterHubProjections(hub.NewHub()))
}

func TestContainer_RecoverOrphans_EmptyIsNoOp(t *testing.T) {
	c := repositories.New(
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		ax[domain.AgentRun](t),
		ax[domain.ReviewThread](t),
	)
	assert.NotPanics(t, func() { c.RecoverOrphans(context.Background()) })
}

func TestContainer_RecoverOrphans_WithItems(t *testing.T) {
	axChat := ax[domain.Chat](t)
	axRun := ax[domain.AgentRun](t)
	c := repositories.New(
		ax[domain.Workspace](t),
		axChat,
		axRun,
		ax[domain.ReviewThread](t),
	)
	assert.NotPanics(t, func() { c.RecoverOrphans(context.Background()) })
}
