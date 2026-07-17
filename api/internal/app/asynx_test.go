package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestNewAsynx_BuildsInstance(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := eventsqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := newAsynx[domain.Workspace](es, ss)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	assert.NotNil(t, ax)
}

func TestNewAsynx_NilStore_ReturnsError(t *testing.T) {
	_, err := newAsynx[domain.Workspace](nil, nil)
	assert.Error(t, err)
}

func TestNewAsynx_NilSnapshotStore_ReturnsError(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	_, err = newAsynx[domain.Workspace](es, nil)
	assert.Error(t, err)
}
