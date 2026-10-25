package chat

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// askedLock is the chat locks, telling asked of every chat a write asks for.
type askedLock struct {
	*ingress.Locks
	asked chan<- string
}

func (l askedLock) Lock(ctx context.Context, chatID string) (func(), error) {
	l.asked <- chatID
	return l.Locks.Lock(ctx, chatID)
}

// openAsk is a ledger holding one open ask, ask-1 of chat-1.
func openAsk(t *testing.T) *ledgerstore.Store {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	l, err := ledgerstore.Open(db, time.Now)
	require.NoError(t, err)
	asked, err := ledger.NewRow(ledger.KindAsk, "T-1", "", ledger.Ask{AskID: "ask-1", Event: "permission"})
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), "chat-1", asked)
	require.NoError(t, err)
	return l
}

// An answer is written only while its relay still waits on it: one whose relay
// was gone by its turn at the chat's lock leaves the ask to that relay's close.
func TestAskResolver_AnAnswerNoRelayWaitsOnIsNotWritten(t *testing.T) {
	l := openAsk(t)
	r := askResolver{writes: ingress.NewWriter(l, ingress.NewLocks())}

	written, err := r.resolve(t.Context(), "chat-1", "ask-1", ledger.ResolutionAnswered, []string{"allow"},
		func() bool { return false })

	require.NoError(t, err)
	assert.False(t, written)
	rows, err := l.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	assert.Len(t, rows, 1, "the ask stays open")
}

// An ask resolves after the hook holding its chat: the resolution waits for
// the chat's lock, so it never lands between that hook's check and its rows.
func TestAskResolver_ResolvesAfterTheHookHoldingTheChat(t *testing.T) {
	for name, resolve := range map[string]func(ctx context.Context, r askResolver) error{
		"a person answers it": func(ctx context.Context, r askResolver) error {
			_, err := r.resolve(ctx, "chat-1", "ask-1", ledger.ResolutionAnswered, []string{"allow"}, always)
			return err
		},
		"its relay hangs up": func(ctx context.Context, r askResolver) error {
			return r.ResolveAsk(ctx, "chat-1", "ask-1", ledger.ResolutionProceeded)
		},
	} {
		t.Run(name, func(t *testing.T) {
			l := openAsk(t)
			locks, asking := ingress.NewLocks(), make(chan string, 1)
			r := askResolver{writes: ingress.NewWriter(l, askedLock{Locks: locks, asked: asking})}
			unlock, err := locks.Lock(t.Context(), "chat-1") // a hook mid-record
			require.NoError(t, err)

			done := make(chan error, 1)
			go func() { done <- resolve(context.Background(), r) }()
			select {
			case <-asking:
			case err := <-done:
				done <- err
			}
			said, err := ledger.NewRow(ledger.KindAssistantText, "T-1", "", ledger.AssistantText{MessageID: "m-1", Text: "said"})
			require.NoError(t, err)
			_, _, err = l.Append(t.Context(), "chat-1", said)
			require.NoError(t, err)
			unlock()
			require.NoError(t, <-done)

			rows, err := l.Read(t.Context(), "chat-1", 0, 0)
			require.NoError(t, err)
			kinds := make([]ledger.Kind, 0, len(rows))
			for _, r := range rows {
				kinds = append(kinds, r.Kind)
			}
			assert.Equal(t, []ledger.Kind{ledger.KindAsk, ledger.KindAssistantText, ledger.KindAskResolved}, kinds)
		})
	}
}
