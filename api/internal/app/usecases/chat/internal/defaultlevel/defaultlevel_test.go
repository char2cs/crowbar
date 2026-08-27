package defaultlevel_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/defaultlevel"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newTable(t *testing.T) *defaultlevel.DefaultLevel {
	t.Helper()
	prefs, err := storesqlite.New[domain.AgentPermissionDefault, string](":memory:")
	require.NoError(t, err)
	return defaultlevel.New(defaultlevel.Deps{Prefs: prefs})
}

func TestGet_UnsetFallsBackToFullAuto(t *testing.T) {
	t.Parallel()
	table := newTable(t)

	level, err := table.Get(t.Context())

	require.NoError(t, err)
	assert.Equal(t, permission.FullAuto, level,
		"the shipped default is full-auto until a user has ever changed it in Settings")
}

func TestSet_ThenGetRoundTrips(t *testing.T) {
	t.Parallel()
	table := newTable(t)

	require.NoError(t, table.Set(t.Context(), permission.Guarded))
	level, err := table.Get(t.Context())

	require.NoError(t, err)
	assert.Equal(t, permission.Guarded, level)
}

func TestSet_RejectsAnUnknownLevel(t *testing.T) {
	t.Parallel()
	table := newTable(t)

	err := table.Set(t.Context(), permission.Level("yolo"))

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}
