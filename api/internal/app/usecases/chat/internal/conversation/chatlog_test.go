package conversation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// speaker is in-package, so this test is too. It is the one piece of the ledger
// that is DISPLAY rather than data, and the harness case is the reason it exists:
// an injected prompt rendered as "user" would let a model reading its own handoff
// mistake the harness for a person.
func TestSpeaker_AttributesAVendorReplyToItsProvider(t *testing.T) {
	for _, tc := range []struct {
		name string
		turn domain.LedgerTurn
		want string
	}{
		{"assistant with a provider", domain.LedgerTurn{Role: "assistant", Provider: "claude"}, "assistant (claude)"},
		{"assistant with no provider", domain.LedgerTurn{Role: "assistant"}, "assistant"},
		{"a user is just the user", domain.LedgerTurn{Role: "user", Provider: "claude"}, "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, speaker(tc.turn))
		})
	}
}

func TestSpeaker_NeverRendersANonUserTurnAsTheUser(t *testing.T) {
	for _, tc := range []struct {
		name string
		turn domain.LedgerTurn
		want string
	}{
		{"harness with a provider", domain.LedgerTurn{Role: "harness", Provider: "codex"}, "codex harness (injected, NOT the user)"},
		{"harness with no provider", domain.LedgerTurn{Role: "harness"}, "harness (injected, NOT the user)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, speaker(tc.turn))
			assert.NotEqual(t, "user", speaker(tc.turn))
		})
	}
}

// askedLock is the chat locks, telling asked of every chat a write asks for.
type askedLock struct {
	*ingress.Locks
	asked chan<- string
}

func (l askedLock) Lock(ctx context.Context, chatID string) (func(), error) {
	l.asked <- chatID
	return l.Locks.Lock(ctx, chatID)
}

// A note lands after the hook holding its chat: it waits for the chat's lock,
// so it never lands between that hook's check and its rows.
func TestAppendNote_LandsAfterTheHookHoldingTheChat(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	l, err := ledgerstore.Open(db, time.Now)
	require.NoError(t, err)
	locks, asking := ingress.NewLocks(), make(chan string, 1)
	c := &Conversations{writes: ingress.NewWriter(l, askedLock{Locks: locks, asked: asking})}
	unlock, err := locks.Lock(t.Context(), "chat-1") // a hook mid-record
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- c.appendNote(context.Background(), "chat-1", "a note") }()
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
	require.Len(t, rows, 2)
	assert.Equal(t, ledger.KindAssistantText, rows[0].Kind)
	assert.Equal(t, ledger.KindUserMessage, rows[1].Kind, "the note follows the hook's row")
}
