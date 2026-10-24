package ledger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// A view holds the chat's writer while it runs, so no commit lands inside it,
// and gives it back when it returns.
func TestStore_ViewHoldsTheChatsWriter(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := Open(db, time.Now)
	require.NoError(t, err)

	require.NoError(t, s.View(t.Context(), "c1", func(ledger.State) error {
		assert.False(t, s.writer("c1").mu.TryLock(), "a commit waits for the view")
		return nil
	}))
	w := s.writer("c1")
	require.True(t, w.mu.TryLock(), "the view gave the writer back")
	w.mu.Unlock()
}
