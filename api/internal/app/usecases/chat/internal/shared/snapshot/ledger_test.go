package snapshot_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// boundTo is chat c1's ledger after Crowbar bound each provider's session, in order.
func boundTo(t *testing.T, sessions ...[2]string) ledger.State {
	t.Helper()
	st := ledger.State{ChatID: "c1"}
	for i, ps := range sessions {
		r, err := ledger.Assign(st, ps[0], ps[1])
		require.NoError(t, err)
		r.Seq, r.At = int64(i+1), t0
		st.Apply(r)
	}
	return st
}

func TestSnapshots_LedgerFactsBumpTheVersionOnlyWhenTheyChange(t *testing.T) {
	ctx := context.Background()
	s, rec, _ := newOwner(t, &reader{chats: map[string]domain.Chat{"c1": {ID: "c1", ProviderID: "p1"}}})
	before, err := s.Get(ctx, "c1")
	require.NoError(t, err)

	facts := snapshot.FactsOf(boundTo(t, [2]string{"p1", "S1"}))
	s.ApplyLedger(ctx, "c1", facts)
	frame := rec.last(t)
	assert.Equal(t, snapshot.KindSnapshot, frame.Kind)
	assert.Greater(t, frame.Snapshot.Version, before.Version)
	c, ok := frame.Snapshot.Conversation()
	require.True(t, ok)
	assert.Equal(t, ledger.Conversation{
		Provider: "p1", SessionID: "S1", State: ledger.ConversationBound, BoundAt: t0, Cause: ledger.CauseAssigned,
	}, c)

	s.ApplyLedger(ctx, "c1", snapshot.FactsOf(boundTo(t, [2]string{"p1", "S1"})))
	assert.Len(t, rec.frames, 1, "the same facts announce nothing")
}

func TestSnapshots_AChatWithoutFactsReadsItsLedgerWithTheLockReleased(t *testing.T) {
	ctx := context.Background()
	rd := &reader{chats: map[string]domain.Chat{"c1": {ID: "c1", ProviderID: "p1"}}}
	s, _, _ := newOwner(t, rd)
	stale := boundTo(t, [2]string{"p1", "S1"})
	published := snapshot.FactsOf(boundTo(t, [2]string{"p1", "S1"}, [2]string{"p1", "S2"}))
	rd.ledger = func(string) (ledger.State, error) {
		s.ApplyLedger(ctx, "c1", published) // the writer publishing while this read is in flight
		return stale, nil
	}

	got, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	c, _ := got.Conversation()
	assert.Equal(t, "S2", c.SessionID, "an older read never replaces what the writer published")

	rd.ledger = func(string) (ledger.State, error) { return ledger.State{}, errors.New("disk gone") }
	again, err := s.Get(ctx, "c1")
	require.NoError(t, err, "facts once held are not read again")
	assert.Equal(t, got.Version, again.Version)

	_, err = s.Get(ctx, "c2")
	require.Error(t, err)
}

// Held is what a ledger subscriber reads under the chat's writer: the ledger is
// never read, so it cannot wait on that writer.
func TestSnapshots_HeldNeverReadsTheLedger(t *testing.T) {
	ctx := context.Background()
	rd := &reader{chats: map[string]domain.Chat{"c1": {ID: "c1", WorkspaceID: "ws1"}}}
	rd.ledger = func(string) (ledger.State, error) {
		t.Error("Held read the ledger")
		return ledger.State{}, nil
	}
	s, _, _ := newOwner(t, rd)

	got, err := s.Held(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "ws1", got.Chat.WorkspaceID)
	_, err = s.Held(ctx, "c2")
	require.Error(t, err)
}

func TestSnapshots_TheConversationIsTheLiveRunnersProvidersElseTheChats(t *testing.T) {
	ctx := context.Background()
	rd := &reader{
		chats: map[string]domain.Chat{"c1": {ID: "c1", ProviderID: "p1"}},
		ledger: func(string) (ledger.State, error) {
			return boundTo(t, [2]string{"p1", "S1"}, [2]string{"p2", "X1"}), nil
		},
	}
	s, _, _ := newOwner(t, rd)

	dormant, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	c, _ := dormant.Conversation()
	assert.Equal(t, "S1", c.SessionID)
	assert.Len(t, dormant.Ledger.Conversations, 2)

	live := runnerOn("r1", "c1", t0)
	live.ProviderID = "p2"
	s.ApplyRunner(ctx, live, live, 1, "started")
	onP2, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	c, _ = onP2.Conversation()
	assert.Equal(t, "X1", c.SessionID)

	live.ProviderID = "p3"
	s.ApplyRunner(ctx, live, live, 2, "started")
	onP3, err := s.Get(ctx, "c1")
	require.NoError(t, err)
	_, ok := onP3.Conversation()
	assert.False(t, ok, "a provider the chat never bound has no conversation")
}
