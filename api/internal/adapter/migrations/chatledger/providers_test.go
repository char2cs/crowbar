package chatledger

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func storeDir(t *testing.T, h home.Home) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(h.Store(), 0o700))
	return h.Store()
}

func TestNewestProviders_TheNewestOfConversationsSwitchesAndPlacementsWins(t *testing.T) {
	h := testutil.FixtureHome(t)
	at := func(m int) time.Time { return time.Date(2026, 9, 1, 12, m, 0, 0, time.UTC) }

	runner := writable(t, filepath.Join(storeDir(t, h), RunnerDB), ConversationsDDL, PlacementsDDL)
	for _, c := range []struct {
		chat, provider string
		at             time.Time
	}{{"switched", "claude", at(1)}, {"placed", "codex", at(3)}, {"tied", "claude", at(1)}} {
		_, err := runner.ExecContext(t.Context(), "INSERT INTO agent_chat_conversations (chat_id, session_id, provider_id, last_active_at) VALUES (?, ?, ?, ?)",
			c.chat, "s-"+c.chat, c.provider, c.at)
		require.NoError(t, err)
	}
	for _, p := range []struct {
		chat, provider string
		at             time.Time
	}{{"switched", "claude", at(0)}, {"placed", "claude", at(5)}, {"tied", "codex", at(1)}} {
		_, err := runner.ExecContext(t.Context(), "INSERT INTO agent_chat_placements (chat_id, provider_id, last_placed_at) VALUES (?, ?, ?)",
			p.chat, p.provider, p.at)
		require.NoError(t, err)
	}
	require.NoError(t, runner.Close())

	activity := writable(t, filepath.Join(h.Store(), ActivityDB), InterruptionsDDL)
	t.Cleanup(func() { require.NoError(t, activity.Close()) })
	for _, in := range []struct{ key, chat, kind, detail string }{
		{"k1", "switched", providerSwitched, "codex"},
		{"k2", "only-switched", providerSwitched, "claude"},
		{"k3", "only-switched", "stopped", "codex"},
	} {
		_, err := activity.ExecContext(t.Context(), "INSERT INTO agent_interruptions (key, chat_id, kind, detail, at) VALUES (?, ?, ?, ?, ?)",
			in.key, in.chat, in.kind, in.detail, at(2))
		require.NoError(t, err)
	}

	got, err := NewestProviders(context.Background(), h)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"switched":      "codex",  // the switch at :02 is newer than the conversation and the placement
		"placed":        "claude", // the placement at :05 is newer than the conversation at :03
		"tied":          "claude", // a tie keeps what was read first
		"only-switched": "claude", // an interruption of another kind names no provider
	}, got)
}

func TestNewestProviders_MissingDatabasesAreEmpty(t *testing.T) {
	h := testutil.FixtureHome(t)

	got, err := NewestProviders(context.Background(), h)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NoDirExists(t, h.Store(), "reading created nothing")

	require.NoError(t, writable(t, filepath.Join(storeDir(t, h), ActivityDB)).Close())
	got, err = NewestProviders(context.Background(), h)
	require.NoError(t, err)
	assert.Empty(t, got, "a database without the legacy tables is empty history")
}

// A history it cannot read halts the step: reading it as empty would leave
// those chats on the provider their history disagrees with.
func TestNewestProviders_AHistoryItCannotReadFails(t *testing.T) {
	for name, seed := range map[string]func(t *testing.T, h home.Home){
		"not a database": func(t *testing.T, h home.Home) {
			t.Helper()
			require.NoError(t, os.WriteFile(filepath.Join(storeDir(t, h), RunnerDB), []byte("not a database"), 0o600))
		},
		"a store that is not a directory": func(t *testing.T, h home.Home) {
			t.Helper()
			require.NoError(t, os.MkdirAll(filepath.Dir(h.Store()), 0o700))
			require.NoError(t, os.WriteFile(h.Store(), nil, 0o600))
		},
		"a table of another shape": func(t *testing.T, h home.Home) {
			t.Helper()
			db := writable(t, filepath.Join(storeDir(t, h), RunnerDB),
				"CREATE TABLE agent_chat_placements (chat_id TEXT, provider_id TEXT)")
			require.NoError(t, db.Close())
		},
		"a timestamp that is not one": func(t *testing.T, h home.Home) {
			t.Helper()
			db := writable(t, filepath.Join(storeDir(t, h), RunnerDB), PlacementsDDL)
			_, err := db.ExecContext(t.Context(), "INSERT INTO agent_chat_placements (chat_id, provider_id, last_placed_at) VALUES ('c1', 'claude', 'yesterday')")
			require.NoError(t, err)
			require.NoError(t, db.Close())
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := testutil.FixtureHome(t)
			seed(t, h)

			_, err := NewestProviders(context.Background(), h)

			assert.Error(t, err)
		})
	}
}
