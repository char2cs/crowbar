package manifestfetch_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/manifestfetch"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newTable(t *testing.T) *manifestfetch.ManifestFetch {
	t.Helper()
	prefs, err := storesqlite.New[domain.AgentModelManifestFetch, string](":memory:")
	require.NoError(t, err)
	return manifestfetch.New(manifestfetch.Deps{Prefs: prefs})
}

func TestGet_UnsetDefaultsToEnabled(t *testing.T) {
	t.Parallel()
	table := newTable(t)

	enabled, err := table.Get(t.Context())

	require.NoError(t, err)
	assert.True(t, enabled, "the shipped default is ON until a user has ever changed it in Settings")
}

func TestSet_ThenGetRoundTrips(t *testing.T) {
	t.Parallel()
	table := newTable(t)

	require.NoError(t, table.Set(t.Context(), false))
	enabled, err := table.Get(t.Context())

	require.NoError(t, err)
	assert.False(t, enabled)
}

func TestGet_NoStoreAtAllDefaultsToEnabled(t *testing.T) {
	t.Parallel()
	table := manifestfetch.New(manifestfetch.Deps{})

	enabled, err := table.Get(t.Context())

	require.NoError(t, err)
	assert.True(t, enabled, "every test fixture that leaves ModelManifestFetchPrefs nil must not panic or dial out")
}

func TestSet_NoStoreAtAllRefuses(t *testing.T) {
	t.Parallel()
	table := manifestfetch.New(manifestfetch.Deps{})

	err := table.Set(t.Context(), false)

	require.ErrorIs(t, err, manifestfetch.ErrNoStore)
}
