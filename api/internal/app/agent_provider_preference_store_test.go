package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestAgentProviderPreferenceStore_RoundTrip proves the generic sqlite store
// persists an AgentProviderPreference keyed by provider id: the table
// auto-migrates, FindAll lists every saved row, and FindByKey resolves one back
// with its Priority/Disabled fields intact.
func TestAgentProviderPreferenceStore_RoundTrip(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	stores, err := newGORMStores(db)
	require.NoError(t, err)

	ctx := context.Background()
	s := stores.AgentProviderPreferences

	require.NoError(t, s.Save(ctx, domain.AgentProviderPreference{ProviderID: "codex", Priority: 0, Disabled: false}))
	require.NoError(t, s.Save(ctx, domain.AgentProviderPreference{ProviderID: "claude", Priority: 1, Disabled: true}))

	all, err := s.FindAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)

	got, err := s.FindByKey(ctx, "claude")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Disabled)
	assert.Equal(t, 1, got.Priority)

	// A replace (same key, new values) upserts rather than duplicating.
	require.NoError(t, s.Save(ctx, domain.AgentProviderPreference{ProviderID: "claude", Priority: 5, Disabled: false}))
	all, err = s.FindAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)

	got, err = s.FindByKey(ctx, "claude")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.Disabled)
	assert.Equal(t, 5, got.Priority)

	// Delete drops the row; a subsequent FindByKey returns nil, nil.
	require.NoError(t, s.Delete(ctx, "claude"))
	gone, err := s.FindByKey(ctx, "claude")
	require.NoError(t, err)
	assert.Nil(t, gone)
}
