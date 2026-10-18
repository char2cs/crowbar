package chatledger

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestLiveRunners_ListsTheRunnersStillOnAChat(t *testing.T) {
	h := testutil.FixtureHome(t)
	db := writable(t, filepath.Join(storeDir(t, h), RunnerDB), RunnersDDL)
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	for _, r := range []struct {
		id, chat, provider string
		started            time.Time
	}{
		{"r-later", "c2", "codex", at.Add(time.Minute)},
		{"r-first", "c1", "claude", at},
		{"r-parked", "", "claude", at},
		{"r-unnamed", "c3", "", at},
	} {
		_, err := db.ExecContext(t.Context(),
			"INSERT INTO agent_runners (id, current_chat_id, provider_id, started_at) VALUES (?, ?, ?, ?)",
			r.id, r.chat, r.provider, r.started)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())

	got, err := LiveRunners(context.Background(), h)

	require.NoError(t, err)
	assert.Equal(t, []LegacyRunner{
		{ID: "r-first", ChatID: "c1", ProviderID: "claude"},
		{ID: "r-later", ChatID: "c2", ProviderID: "codex"},
	}, got, "a runner on no chat, or of no provider, has nothing to end")
}

func TestLiveRunners_AMissingStoreIsNone(t *testing.T) {
	h := testutil.FixtureHome(t)

	got, err := LiveRunners(context.Background(), h)

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NoDirExists(t, h.Store(), "reading created nothing")
}
