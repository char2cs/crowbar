package transcript_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/transcript"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

const chatID = "chat-1"

// book is a chat's ledger with the projector subscribed to it, as the usecase
// wires them. Its clock ticks a second a commit.
type book struct {
	t      *testing.T
	ledger *ledgerstore.Store
	p      *transcript.Projector
	mu     sync.Mutex
	// changed is every message a publication reported changed.
	changed []domain.LedgerMessage
}

func newBook(t *testing.T) *book {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	var mu sync.Mutex
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	store, err := ledgerstore.Open(db, func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(time.Second)
		return now
	})
	require.NoError(t, err)
	b := &book{t: t, ledger: store, p: transcript.New(store)}
	store.Subscribe(func(id string, rows []ledger.Row, _ ledger.State) {
		changed := b.p.Apply(id, rows)
		b.mu.Lock()
		defer b.mu.Unlock()
		b.changed = append(b.changed, changed...)
	})
	return b
}

func mk[B any](t *testing.T, kind ledger.Kind, turnID, scope string, body B) ledger.Row {
	t.Helper()
	r, err := ledger.NewRow(kind, turnID, scope, body)
	require.NoError(t, err)
	return r
}

func (b *book) add(rows ...ledger.Row) {
	b.t.Helper()
	_, _, err := b.ledger.Append(b.t.Context(), chatID, rows...)
	require.NoError(b.t, err)
}

// open starts turnID on a prompt typed into the CLI.
func (b *book) open(turnID, text string) {
	b.t.Helper()
	b.add(
		mk(b.t, ledger.KindUserMessage, turnID, "", ledger.UserMessage{Text: text, Origin: ledger.OriginProvider}),
		mk(b.t, ledger.KindTurnStarted, turnID, "", ledger.TurnStarted{Provider: "p1", RunnerID: "run-1", SessionID: "s-1"}),
	)
}

func (b *book) say(scope, messageID, text string) {
	b.t.Helper()
	b.add(mk(b.t, ledger.KindAssistantText, "T1", scope,
		ledger.AssistantText{MessageID: messageID, Index: ledger.IndexComplete, Text: text}))
}

func (b *book) end() {
	b.t.Helper()
	b.add(mk(b.t, ledger.KindTurnEnded, "T1", "", ledger.TurnEnded{Status: "done", RunnerID: "run-1"}))
}

func (b *book) messages() []domain.LedgerMessage {
	b.t.Helper()
	page, err := b.p.Messages(b.t.Context(), chatID, 0, 0, 1000)
	require.NoError(b.t, err)
	return page.Items
}

func (b *book) activity() transcript.Activity {
	b.t.Helper()
	act, err := b.p.Activity(b.t.Context(), chatID)
	require.NoError(b.t, err)
	return act
}

func texts(messages []domain.LedgerMessage) []string {
	out := make([]string, 0, len(messages))
	for _, m := range messages {
		out = append(out, m.Text)
	}
	return out
}

func TestProjector_ReadsWhatTheLedgerHolds(t *testing.T) {
	b := newBook(t)
	b.open("T1", "hello")
	b.say("", "m1", "hi")
	b.end()

	assert.Equal(t, []string{"hello", "hi"}, texts(b.messages()))
	assert.Equal(t, "msg-m1", b.messages()[1].ID, "a message is listed under the id its live frames name")
}

// A chat nobody read is left to its first read; the publication does not
// build it.
func TestProjector_ApplyOnAChatNotReadDoesNothing(t *testing.T) {
	b := newBook(t)
	b.open("T1", "hello")

	b.mu.Lock()
	defer b.mu.Unlock()
	assert.Empty(t, b.changed)
}

func TestProjector_ApplyReportsTheMessagesItChanged(t *testing.T) {
	b := newBook(t)
	b.open("T1", "hello")
	b.messages()

	b.say("", "m1", "hi")

	b.mu.Lock()
	defer b.mu.Unlock()
	require.Len(t, b.changed, 1)
	assert.Equal(t, "hi", b.changed[0].Text)
}

// Rows that do not follow the cached head drop the chat, and its next read
// refills it from the ledger.
func TestProjector_RowsThatDoNotFollowDropTheChat(t *testing.T) {
	b := newBook(t)
	b.open("T1", "hello")
	b.messages()

	assert.Nil(t, b.p.Apply(chatID, []ledger.Row{{Seq: 99, Kind: ledger.KindMarker}}))
	assert.Nil(t, b.p.Apply(chatID, nil))

	assert.Equal(t, []string{"hello"}, texts(b.messages()))
}

func TestProjector_RebuildsFromRowsOnAMiss(t *testing.T) {
	b := newBook(t)
	b.open("T1", "hello")
	b.say("", "m1", "hi")
	first := b.messages()

	for i := range 64 {
		_, err := b.p.Messages(t.Context(), fmt.Sprintf("other-%d", i), 0, 0, 10)
		require.NoError(t, err)
	}

	assert.Equal(t, first, b.messages(), "the evicted chat is rebuilt identically from its rows")
}

func TestProjector_ForgetDropsTheChat(t *testing.T) {
	b := newBook(t)
	b.open("T1", "hello")
	b.messages()
	b.p.Forget(chatID)
	b.p.Forget("never-read")
	b.say("", "m1", "hi")

	b.mu.Lock()
	assert.Empty(t, b.changed, "a forgotten chat is not advanced")
	b.mu.Unlock()
	assert.Equal(t, []string{"hello", "hi"}, texts(b.messages()))
}

// A fill races every publication: it runs under the chat's writer, so the rows
// it reads are the head and nothing published after can be lost.
func TestProjector_AFillRacingAPublicationIsNeverStale(t *testing.T) {
	b := newBook(t)
	b.open("T1", "hello")

	const said = 40
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range said {
			b.say("", fmt.Sprintf("m%d", i), fmt.Sprintf("line %d", i))
		}
	}()
	for {
		b.p.Forget(chatID)
		_, err := b.p.Messages(t.Context(), chatID, 0, 0, 1000)
		require.NoError(t, err)
		select {
		case <-done:
			assert.Len(t, b.messages(), said+1, "every row published while the fill ran is read")
			return
		default:
		}
	}
}

func TestProjector_ChatTurnsAreInTheOrderShown(t *testing.T) {
	b := newBook(t)
	b.open("T1", "first")
	b.add(mk(t, ledger.KindAssistantText, "T1", "", ledger.AssistantText{MessageID: "m1", Index: 0, Text: "begun "}))
	b.say("", "m2", "second")
	b.add(mk(t, ledger.KindAssistantText, "T1", "", ledger.AssistantText{MessageID: "m1", Index: 1, Final: true, Text: "early"}))

	turns, err := b.p.ChatTurns(t.Context(), chatID)
	require.NoError(t, err)
	got := make([]string, 0, len(turns))
	for _, turn := range turns {
		got = append(got, turn.Text)
	}
	assert.Equal(t, []string{"first", "begun early", "second"}, got,
		"a message is shown where it began, though it settled after the next")
}

func TestProjector_PagesMessagesBySequence(t *testing.T) {
	b := newBook(t)
	b.open("T1", "u1")
	for i := range 4 {
		b.say("", fmt.Sprintf("m%d", i), fmt.Sprintf("a%d", i))
	}
	all := b.messages()
	require.Len(t, all, 5)

	newest, err := b.p.Messages(t.Context(), chatID, 0, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"a2", "a3"}, texts(newest.Items))
	assert.True(t, newest.HasMore)
	assert.Equal(t, all[3].Sequence, newest.OldestCursor)
	assert.Equal(t, all[4].Sequence, newest.Cursor)

	older, err := b.p.Messages(t.Context(), chatID, 0, newest.OldestCursor, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"a0", "a1"}, texts(older.Items))

	after, err := b.p.Messages(t.Context(), chatID, all[1].Sequence, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"a1", "a2"}, texts(after.Items))
	assert.True(t, after.HasMore)

	none, err := b.p.Messages(t.Context(), chatID, all[4].Sequence, 0, 2)
	require.NoError(t, err)
	assert.NotNil(t, none.Items)
	assert.Empty(t, none.Items)
	assert.False(t, none.HasMore)
}

// Messages settled by one row share its Sequence, and a page never splits them.
func TestProjector_APageNeverSplitsMessagesOfOneSequence(t *testing.T) {
	b := newBook(t)
	b.open("T1", "u1")
	b.add(
		mk(t, ledger.KindAssistantText, "T1", "", ledger.AssistantText{MessageID: "m1", Index: 0, Text: "one"}),
		mk(t, ledger.KindAssistantText, "T1", "", ledger.AssistantText{MessageID: "m2", Index: 0, Text: "two"}),
	)
	b.end()

	page, err := b.p.Messages(t.Context(), chatID, 0, 0, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"one", "two"}, texts(page.Items))
	from, err := b.p.Messages(t.Context(), chatID, b.messages()[0].Sequence, 0, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"one", "two"}, texts(from.Items))
}

func TestProjector_ARefusedReadIsAnError(t *testing.T) {
	b := newBook(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := b.p.Messages(ctx, chatID, 0, 0, 10)
	require.Error(t, err)
	_, err = b.p.ChatTurns(ctx, chatID)
	require.Error(t, err)
	_, err = b.p.Activity(ctx, chatID)
	require.Error(t, err)
	_, err = b.p.PendingChoices(ctx, chatID)
	require.Error(t, err)
}
