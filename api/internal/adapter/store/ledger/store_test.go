package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

var fixedNow = time.Date(2026, 10, 18, 9, 30, 0, 123456000, time.UTC)

func clock() time.Time { return fixedNow }

func openDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := storesqlite.OpenDB(path)
	require.NoError(t, err)
	t.Cleanup(func() { closeDB(t, db) })
	return db
}

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func newStore(t *testing.T) *ledgerstore.Store {
	t.Helper()
	s, err := ledgerstore.Open(openDB(t, filepath.Join(t.TempDir(), ledgerstore.FileName)), clock)
	require.NoError(t, err)
	return s
}

func mustRow[B any](t *testing.T, kind ledger.Kind, turnID string, body B) ledger.Row {
	t.Helper()
	r, err := ledger.NewRow(kind, turnID, "", body)
	require.NoError(t, err)
	return r
}

func prompt(t *testing.T, requestID, text string) ledger.Row {
	t.Helper()
	return mustRow(t, ledger.KindUserMessage, "T-"+requestID, ledger.UserMessage{
		RequestID: requestID, Text: text, Origin: ledger.OriginCrowbar, Delivery: ledger.DeliveryRestart,
	})
}

func settle(t *testing.T, requestID string) ledger.Row {
	t.Helper()
	return mustRow(t, ledger.KindPromptSettled, "", ledger.PromptSettled{
		RequestID: requestID, Reason: ledger.ReasonRunnerExited,
	})
}

func started(t *testing.T, requestID string) ledger.Row {
	t.Helper()
	return mustRow(t, ledger.KindTurnStarted, "T-"+requestID, ledger.TurnStarted{RequestID: requestID})
}

func seqs(rows []ledger.Row) []int64 {
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Seq)
	}
	return out
}

func TestStore_AppendAssignsContiguousSeqsFromOne(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	rows, st, err := s.Append(ctx, "c1", prompt(t, "r1", "a"), started(t, "r1"), prompt(t, "r2", "b"))
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2, 3}, seqs(rows))
	assert.Equal(t, int64(3), st.Head())
	for _, r := range rows {
		assert.Equal(t, "c1", r.ChatID)
		assert.True(t, fixedNow.Equal(r.At), "At is the store clock's")
	}

	other, _, err := s.Append(ctx, "c2", prompt(t, "r9", "z"))
	require.NoError(t, err)
	assert.Equal(t, []int64{1}, seqs(other))

	read, err := s.Read(ctx, "c1", 0, 0)
	require.NoError(t, err)
	require.Len(t, read, 3)
	assert.True(t, fixedNow.Equal(read[0].At), "At round-trips through the table")
	assert.JSONEq(t, string(rows[0].Body), string(read[0].Body))
}

func TestStore_ConcurrentAppendsGetContiguousSeqsPublishedInOrder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	var mu sync.Mutex
	var recorded []int64
	s.Subscribe(func(chatID string, rows []ledger.Row, _ ledger.State) {
		mu.Lock()
		defer mu.Unlock()
		recorded = append(recorded, seqs(rows)...)
	})

	rows := make([]ledger.Row, 50)
	for i := range rows {
		rows[i] = mustRow(t, ledger.KindUserMessage, fmt.Sprintf("T%d", i),
			ledger.UserMessage{Text: "typed", Origin: ledger.OriginProvider})
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(rows))
	for _, r := range rows {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.Append(ctx, "c1", r)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	want := make([]int64, 50)
	for i := range want {
		want[i] = int64(i + 1)
	}
	assert.Equal(t, want, recorded)
	read, err := s.Read(ctx, "c1", 0, 0)
	require.NoError(t, err)
	assert.Len(t, read, 50)
}

func TestStore_DoSeesTheStateItDecidesAgainst(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	start := make(chan struct{})
	type outcome struct {
		rows []ledger.Row
		err  error
	}
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, r := range []ledger.Row{prompt(t, "r1", "text"), prompt(t, "r2", "text")} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rows, _, err := s.Do(ctx, "c1", func(st ledger.State) ([]ledger.Row, error) {
				if len(st.PendingPrompts()) > 0 {
					return nil, nil
				}
				return []ledger.Row{r}, nil
			})
			results <- outcome{rows, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	committed := 0
	for got := range results {
		require.NoError(t, got.err)
		committed += len(got.rows)
	}
	assert.Equal(t, 1, committed, "exactly one decision may see an empty chat")
	st, err := s.State(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), st.Head())
}

func TestStore_DoWithNoRowsCommitsNothingAndPublishesNothing(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	published := 0
	s.Subscribe(func(string, []ledger.Row, ledger.State) { published++ })

	rows, st, err := s.Do(ctx, "c1", func(ledger.State) ([]ledger.Row, error) { return nil, nil })
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.Equal(t, int64(0), st.Head())
	assert.Zero(t, published)

	refused := errors.New("refused")
	_, _, err = s.Do(ctx, "c1", func(ledger.State) ([]ledger.Row, error) { return nil, refused })
	require.ErrorIs(t, err, refused)
	assert.Zero(t, published)
}

func TestStore_SubscriberRunsAfterCommit(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	readBack := make(chan []ledger.Row, 1)
	s.Subscribe(func(chatID string, _ []ledger.Row, st ledger.State) {
		rows, err := s.Read(ctx, chatID, 0, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(2), st.Head())
		readBack <- rows
	})

	_, _, err := s.Append(ctx, "c1", prompt(t, "r1", "a"), started(t, "r1"))
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2}, seqs(<-readBack))
}

func TestStore_RejectsMalformedRowsAtomically(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	malformed := prompt(t, "r2", "b")
	malformed.TurnID = ""

	_, _, err := s.Append(ctx, "c1", prompt(t, "r1", "a"), malformed)
	require.Error(t, err)

	read, err := s.Read(ctx, "c1", 0, 0)
	require.NoError(t, err)
	assert.Empty(t, read)
	st, err := s.State(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(0), st.Head())
	_, ok := st.Receipt("r1")
	assert.False(t, ok)
}

func TestStore_StateSurvivesReopenWithManyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), ledgerstore.FileName)
	ctx := context.Background()
	db, err := storesqlite.OpenDB(path)
	require.NoError(t, err)
	s, err := ledgerstore.Open(db, clock)
	require.NoError(t, err)

	batch := make([]ledger.Row, 0, 100)
	for i := range 2500 {
		id := fmt.Sprintf("r%d", i)
		batch = append(batch, prompt(t, id, "text "+id))
		if i%3 == 0 {
			batch = append(batch, started(t, id))
		} else {
			batch = append(batch, settle(t, id))
		}
		if len(batch) >= 100 {
			_, _, err = s.Append(ctx, "c1", batch...)
			require.NoError(t, err)
			batch = batch[:0]
		}
	}
	_, before, err := s.Append(ctx, "c1", prompt(t, "last", "still pending"))
	require.NoError(t, err)
	require.Equal(t, int64(5001), before.Head())
	closeDB(t, db)

	reopened, err := ledgerstore.Open(openDB(t, path), clock)
	require.NoError(t, err)
	after, err := reopened.State(ctx, "c1")
	require.NoError(t, err)

	assert.Equal(t, before.Head(), after.Head())
	assert.Equal(t, before.PendingPrompts(), after.PendingPrompts())
	for _, id := range []string{"r0", "r1", "r2499"} {
		want, ok := before.Receipt(id)
		require.True(t, ok)
		got, ok := after.Receipt(id)
		require.True(t, ok)
		assert.Equal(t, want, got, id)
	}
}

func TestStore_OpenChatsListsOnlyChatsWithOpenWork(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, err := s.Append(ctx, "c1", prompt(t, "r1", "a"))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c2", prompt(t, "r2", "b"), settle(t, "r2"))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c3", prompt(t, "r3", "c"))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c3", started(t, "r3"))
	require.NoError(t, err)

	open, err := s.OpenChats(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, open)
}

func TestStore_ForgetErasesAChat(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, err := s.Append(ctx, "c1", prompt(t, "r1", "a"))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c2", prompt(t, "r2", "b"))
	require.NoError(t, err)

	require.NoError(t, s.Forget(ctx, "c1"))

	read, err := s.Read(ctx, "c1", 0, 0)
	require.NoError(t, err)
	assert.Empty(t, read)
	st, err := s.State(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(0), st.Head())
	open, err := s.OpenChats(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"c2"}, open)
	kept, err := s.Read(ctx, "c2", 0, 0)
	require.NoError(t, err)
	assert.Len(t, kept, 1)

	again, _, err := s.Append(ctx, "c1", prompt(t, "r3", "c"))
	require.NoError(t, err)
	assert.Equal(t, []int64{1}, seqs(again), "a forgotten chat starts over from seq 1")
}

// boundS is provider's CLI announcing session "S".
func boundS(t *testing.T, provider string) ledger.Row {
	t.Helper()
	return mustRow(t, ledger.KindSessionBound, "", ledger.SessionBound{
		Provider: provider, SessionID: "S", Cause: ledger.CauseAnnounced,
	})
}

func TestStore_ASessionCanBeBoundToOneChatOnly(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, err := s.Append(ctx, "c1", boundS(t, "p"))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c2", prompt(t, "r1", "a"))
	require.NoError(t, err)

	_, st, err := s.Append(ctx, "c2", boundS(t, "p"), settle(t, "r1"))
	require.ErrorIs(t, err, ledgerstore.ErrSessionClaimed)
	assert.Equal(t, int64(1), st.Head(), "the whole Do is refused")
	_, _, err = s.Append(ctx, "c1", boundS(t, "p"))
	require.NoError(t, err, "rebinding a chat's own session is no claim")
	_, _, err = s.Append(ctx, "c2", boundS(t, "q"))
	require.NoError(t, err, "a session is owned per provider")

	owner, ok, err := s.SessionOwner(ctx, "p", "S")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "c1", owner)

	require.NoError(t, s.Forget(ctx, "c1"))
	_, ok, err = s.SessionOwner(ctx, "p", "S")
	require.NoError(t, err)
	assert.False(t, ok, "a forgotten chat owns nothing")
}

// fired reports whether ch is already closed; a commit closes it before Do returns.
func fired(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestStore_WatchIsWokenByTheNextCommitToThatChatOnly(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, err := s.Append(ctx, "c1", prompt(t, "r1", "a"))
	require.NoError(t, err)

	st, changed, err := s.Watch(ctx, "c1")
	require.NoError(t, err)
	assert.Len(t, st.PendingPrompts(), 1)
	_, _, err = s.Append(ctx, "c2", prompt(t, "r2", "b"))
	require.NoError(t, err)
	_, _, err = s.Do(ctx, "c1", func(ledger.State) ([]ledger.Row, error) { return nil, nil })
	require.NoError(t, err)
	assert.False(t, fired(changed), "another chat's commit, or a decision with no rows, changes nothing")

	_, _, err = s.Append(ctx, "c1", started(t, "r1"))
	require.NoError(t, err)
	assert.True(t, fired(changed))
	st, again, err := s.Watch(ctx, "c1")
	require.NoError(t, err)
	assert.Empty(t, st.PendingPrompts())
	assert.False(t, fired(again), "each commit arms a fresh signal")

	require.NoError(t, s.Forget(ctx, "c1"))
	assert.True(t, fired(again), "a forgotten chat wakes its watchers")
}

func TestStore_ReportsAClosedDatabase(t *testing.T) {
	db, err := storesqlite.OpenDB(filepath.Join(t.TempDir(), ledgerstore.FileName))
	require.NoError(t, err)
	s, err := ledgerstore.Open(db, clock)
	require.NoError(t, err)
	ctx := context.Background()
	_, _, err = s.Append(ctx, "warm", prompt(t, "r1", "a"))
	require.NoError(t, err)
	closeDB(t, db)

	_, _, err = s.Append(ctx, "warm", prompt(t, "r2", "b"))
	require.Error(t, err)
	_, err = s.State(ctx, "cold")
	require.Error(t, err)
	_, _, err = s.Watch(ctx, "cold")
	require.Error(t, err)
	_, err = s.OpenChats(ctx)
	require.Error(t, err)
	_, _, err = s.SessionOwner(ctx, "p", "S")
	require.Error(t, err)
	require.Error(t, s.Forget(ctx, "warm"))
	st, err := s.State(ctx, "warm")
	require.NoError(t, err, "a failed write leaves the cached state as it was")
	assert.Equal(t, int64(1), st.Head())

	_, err = ledgerstore.Open(db, clock)
	require.Error(t, err)
}
