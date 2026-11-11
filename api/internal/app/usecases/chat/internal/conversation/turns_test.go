package conversation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// rowBook is a chat's ledger and the record read over it.
type rowBook struct {
	t     *testing.T
	store *ledgerstore.Store
	c     *Conversations
}

func newRowBook(t *testing.T) *rowBook {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	store, err := ledgerstore.Open(t.Context(), db, time.Now)
	require.NoError(t, err)
	return &rowBook{t: t, store: store, c: &Conversations{ledger: store}}
}

func (b *rowBook) add(kind ledger.Kind, turnID, scope string, body any) {
	b.t.Helper()
	r, err := ledger.NewRow(kind, turnID, scope, body)
	require.NoError(b.t, err)
	_, _, err = b.store.Append(b.t.Context(), "chat-1", r)
	require.NoError(b.t, err)
}

// open starts turnID on run-1 of p1 with a person's prompt.
func (b *rowBook) open(turnID, text string) {
	b.add(ledger.KindUserMessage, turnID, "", ledger.UserMessage{Text: text, Origin: ledger.OriginProvider})
	b.add(ledger.KindTurnStarted, turnID, "", ledger.TurnStarted{Provider: "p1", RunnerID: "run-1"})
}

func (b *rowBook) say(turnID, scope, messageID, text string) {
	b.add(ledger.KindAssistantText, turnID, scope, ledger.AssistantText{MessageID: messageID, Index: -1, Text: text})
}

func (b *rowBook) turns() []domain.LedgerTurn {
	b.t.Helper()
	got, err := b.c.ChatTurns(b.t.Context(), "chat-1")
	require.NoError(b.t, err)
	return got
}

// A prompt Crowbar delivered is said once its turn takes it, where that turn
// started, under the id the client sent it with.
func TestChatTurns_ACrowbarPromptIsSaidOnceItsTurnStarts(t *testing.T) {
	b := newRowBook(t)
	b.add(ledger.KindUserMessage, "T1", "", ledger.UserMessage{
		RequestID: "req-1", Text: "do it", Origin: ledger.OriginCrowbar, Delivery: ledger.DeliveryRestart,
	})
	assert.Empty(t, b.turns(), "a prompt not taken yet is not said")
	b.add(ledger.KindTurnEnded, "T1", "", ledger.TurnEnded{Status: "done", RunnerID: "run-1"})
	assert.Empty(t, b.turns(), "nor is one whose turn ended without starting")

	b.add(ledger.KindTurnStarted, "T1", "", ledger.TurnStarted{Provider: "p1", RunnerID: "run-1", SessionID: "s-1"})

	got := b.turns()
	require.Len(t, got, 1)
	assert.Equal(t, domain.LedgerTurn{
		ID: "req-1", Role: domain.TurnRoleUser, Provider: "p1", RunnerID: "run-1", SessionID: "s-1",
		Text: "do it", At: got[0].At,
	}, got[0])
}

func TestChatTurns_AHarnessPromptKeepsItsRoleAndANoteIsCrowbars(t *testing.T) {
	b := newRowBook(t)
	b.add(ledger.KindUserMessage, "T1", "", ledger.UserMessage{Text: "a subagent handed back", Origin: ledger.OriginHarness})
	b.add(ledger.KindTurnStarted, "T1", "", ledger.TurnStarted{Provider: "p1"})
	b.add(ledger.KindUserMessage, "N1", "", ledger.UserMessage{Text: "switched", Origin: ledger.OriginNote})

	got := b.turns()
	require.Len(t, got, 2)
	assert.Equal(t, domain.TurnRoleHarness, got[0].Role)
	assert.Equal(t, "p1", got[0].Provider)
	assert.Equal(t, domain.TurnRoleUser, got[1].Role)
	assert.Equal(t, "crowbar", got[1].Provider)
}

// The reply is said where it began, though it settles later, and carries the
// effort its turn ended on; a subagent's reply is not the conversation's.
func TestChatTurns_AReplyIsSaidWhereItBeganWithItsTurnsEffort(t *testing.T) {
	b := newRowBook(t)
	b.open("T1", "go")
	b.add(ledger.KindAssistantText, "T1", "", ledger.AssistantText{MessageID: "m1", Index: 0, Text: "par"})
	b.open("T2", "and then")
	b.add(ledger.KindAssistantText, "T1", "", ledger.AssistantText{MessageID: "m1", Index: 1, Final: true, Text: "tial"})
	b.say("T1", "A", "sub", "nested")
	b.add(ledger.KindTurnEnded, "T1", "", ledger.TurnEnded{Status: "done", RunnerID: "run-1", Effort: "high"})

	got := b.turns()
	require.Len(t, got, 3)
	assert.Equal(t, []string{"go", "partial", "and then"}, []string{got[0].Text, got[1].Text, got[2].Text})
	assert.Equal(t, "msg-m1", got[1].ID)
	assert.Equal(t, domain.TurnRoleAssistant, got[1].Role)
	assert.Equal(t, "high", got[1].Effort)
}

// A turn whose start never came is its ending runner's, and so of the
// provider that runner ran.
func TestChatTurns_AReplyWithNoStartIsItsRunnersProvider(t *testing.T) {
	b := newRowBook(t)
	b.add(ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{RunnerID: "run-1", Provider: "p1"})
	b.say("T1", "", "m1", "done thing")
	b.add(ledger.KindTurnEnded, "T1", "", ledger.TurnEnded{Status: "done", RunnerID: "run-1"})

	got := b.turns()
	require.Len(t, got, 1)
	assert.Equal(t, "p1", got[0].Provider)
	assert.Equal(t, "run-1", got[0].RunnerID)
}

func TestChatTurns_AFailedTurnIsOneNotice(t *testing.T) {
	b := newRowBook(t)
	b.open("T1", "go")
	b.add(ledger.KindTurnFailed, "T1", "", ledger.TurnFailed{Reason: "rate_limited", Detail: "slow down"})
	b.add(ledger.KindTurnFailed, "T1", "", ledger.TurnFailed{Reason: "again"})
	b.open("T2", "go on")
	b.add(ledger.KindTurnFailed, "T2", "", ledger.TurnFailed{Reason: "rate_limited"})

	got := b.turns()
	require.Len(t, got, 4, "one notice per failed turn")
	assert.Equal(t, domain.TurnRoleNotice, got[1].Role)
	assert.Equal(t, "notice-T1", got[1].ID)
	assert.Equal(t, "p1", got[1].Provider)
	assert.Equal(t, "slow down", got[1].Text, "the detail is what the person reads")
	assert.Equal(t, "rate_limited", got[3].Text, "with none, the reason")
}

// A chat longer than one read is read whole.
func TestChatTurns_ReadsEveryPageOfTheLedger(t *testing.T) {
	b := newRowBook(t)
	for range turnsPage {
		b.add(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerNotification})
	}
	b.open("T1", "past the first page")

	got := b.turns()
	require.Len(t, got, 1)
	assert.Equal(t, "past the first page", got[0].Text)
}
