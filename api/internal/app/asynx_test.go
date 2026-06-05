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
	ax, err := newAsynx[domain.Workspace](es)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	assert.NotNil(t, ax)
}

func TestNewAsynx_NilStore_ReturnsError(t *testing.T) {
	_, err := newAsynx[domain.Workspace](nil)
	assert.Error(t, err)
}
