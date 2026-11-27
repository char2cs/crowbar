package ledger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestStore_ViewHoldsTheChatsWriter(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := Open(t.Context(), db, time.Now)
	require.NoError(t, err)

	require.NoError(t, s.View(t.Context(), "c1", func(ledger.State) error {
		assert.False(t, s.writer("c1").mu.TryLock(), "a commit waits for the view")
		return nil
	}))
	w := s.writer("c1")
	require.True(t, w.mu.TryLock(), "the view gave the writer back")
	w.mu.Unlock()
}

func TestStore_AnImportLeavesNoStateHeld(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := Open(t.Context(), db, time.Now)
	require.NoError(t, err)
	r, err := ledger.NewRow(ledger.KindUserMessage, "T1", "", ledger.UserMessage{Text: "hi", Origin: ledger.OriginProvider})
	require.NoError(t, err)
	r.ChatID, r.Seq, r.At = "c1", 1, time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)

	require.NoError(t, s.Import(t.Context(), "c1", []ledger.Row{r}))

	assert.Nil(t, s.writer("c1").state, "the import's fold is not kept")
	st, err := s.State(t.Context(), "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), st.Head(), "the next touch folds the imported rows")
}
