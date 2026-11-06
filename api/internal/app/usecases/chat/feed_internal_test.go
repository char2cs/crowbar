package chat

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func seqRow(seq int64) ledger.Row { return ledger.Row{ChatID: "c1", Seq: seq, Kind: ledger.KindMarker} }

func seqRows(from, to int64) []ledger.Row {
	var rows []ledger.Row
	for seq := from; seq <= to; seq++ {
		rows = append(rows, seqRow(seq))
	}
	return rows
}

// attaching emits frames as a replay would.
func attaching(during func(), frames ...FeedFrame) func(func(FeedFrame) bool) error {
	return func(emit func(FeedFrame) bool) error {
		if during != nil {
			during()
		}
		for _, f := range frames {
			if !emit(f) {
				return context.Canceled
			}
		}
		return nil
	}
}

// drain reads frames until the feed closes.
func drain(out <-chan FeedFrame) []FeedFrame {
	var frames []FeedFrame
	for f := range out {
		frames = append(frames, f)
	}
	return frames
}

// take reads n frames.
func take(t *testing.T, out <-chan FeedFrame, n int) []FeedFrame {
	t.Helper()
	frames := make([]FeedFrame, 0, n)
	for range n {
		f, ok := <-out
		require.True(t, ok, "the feed closed after %d frames: %+v", len(frames), frames)
		frames = append(frames, f)
	}
	return frames
}

func seqsOf(frames []FeedFrame) []int64 {
	var seqs []int64
	for _, f := range frames {
		if f.Row != nil {
			seqs = append(seqs, f.Row.Seq)
		}
	}
	return seqs
}

func rowFrames(rows []ledger.Row) []FeedFrame {
	frames := make([]FeedFrame, 0, len(rows))
	for _, r := range rows {
		frames = append(frames, rowFrame(r))
	}
	return frames
}

func TestFeeds_AFullBufferResyncsInsteadOfSkipping(t *testing.T) {
	f := newFeeds(2)
	attached := make(chan struct{})
	out := f.open(t.Context(), "c1", 0, attaching(func() { close(attached) }))
	<-attached

	f.rows("c1", seqRows(1, 3))

	frames := drain(out)
	require.Len(t, frames, 3)
	assert.Equal(t, []int64{1, 2}, seqsOf(frames))
	assert.Equal(t, FeedFrame{Type: FeedResync, Seq: 2}, frames[2], "resync from the last row it was sent")
}

// The frame the writer holds is unread too: with row 1 in its hand, two more
// rows overflow a feed of two.
func TestFeeds_TheFrameInTheWritersHandCountsAsUnread(t *testing.T) {
	f := newFeeds(2)
	attached := make(chan struct{})
	out := f.open(t.Context(), "c1", 0, attaching(func() { close(attached) }))
	<-attached
	f.rows("c1", seqRows(1, 1))
	subs := f.subs("c1")
	require.Len(t, subs, 1)
	sub := subs[0]
	require.Eventually(t, func() bool {
		sub.mu.Lock()
		defer sub.mu.Unlock()
		return sub.holding
	}, 10*time.Second, time.Millisecond)

	f.rows("c1", seqRows(2, 3))

	frames := take(t, out, 3)
	assert.Equal(t, []int64{1, 2}, seqsOf(frames))
	require.Equal(t, FeedFrame{Type: FeedResync, Seq: 2}, frames[2])
	_, open := <-out
	assert.False(t, open)
}

func TestFeeds_LiveRowsDuringReplayAreDeliveredOnce(t *testing.T) {
	f := newFeeds(16)
	ready := FeedFrame{Type: FeedReady, Seq: 3}
	commitDuringReplay := func() { f.rows("c1", seqRows(2, 3)); f.rows("c1", seqRows(4, 5)) }
	out := f.open(t.Context(), "c1", 0, attaching(commitDuringReplay, append(rowFrames(seqRows(1, 3)), ready)...))

	frames := take(t, out, 6)

	assert.Equal(t, []int64{1, 2, 3, 4, 5}, seqsOf(frames), "rows 2 and 3 were replayed and committed: sent once")
	assert.Equal(t, ready, frames[3])
}

func TestFeeds_ReadyIsFollowedBySnapshotsThenLiveFrames(t *testing.T) {
	f := newFeeds(16)
	live := ledger.FeedDelta{ID: "m1", Stream: "answer", N: 3, Text: "c"}
	snap := ledger.FeedDelta{ID: "m1", Stream: "answer", N: 2, Text: "ab", Replace: true}
	during := func() {
		f.delta("c1", live)
		f.rows("c1", seqRows(1, 1))
		f.delta("c2", ledger.FeedDelta{ID: "elsewhere"})
	}
	out := f.open(t.Context(), "c1", 0, attaching(during,
		FeedFrame{Type: FeedReady}, FeedFrame{Type: feedDelta, Delta: &snap}))

	frames := take(t, out, 4)

	types := []string{}
	for _, fr := range frames {
		types = append(types, fr.Type)
	}
	assert.Equal(t, []string{FeedReady, feedDelta, feedDelta, feedRow}, types)
	assert.Equal(t, &snap, frames[1].Delta)
	assert.Equal(t, &live, frames[2].Delta)
}

// apply is the client's rule: a delta applies only as its stream's next frame;
// a Replace sets the text and the count.
func apply(frames []FeedFrame) string {
	text, last := "", 0
	for _, f := range frames {
		switch d := f.Delta; {
		case d == nil:
		case d.Replace && d.N >= last:
			text, last = d.Text, d.N
		case d.N == last+1:
			text, last = text+d.Text, d.N
		}
	}
	return text
}

func TestFeeds_ADeltaRacingTheSnapshotNeverDuplicatesText(t *testing.T) {
	d := func(n int, text string) ledger.FeedDelta {
		return ledger.FeedDelta{ID: "m1", Stream: "answer", N: n, Text: text}
	}
	snapshot := func(n int, text string) FeedFrame {
		return FeedFrame{Type: feedDelta, Delta: &ledger.FeedDelta{ID: "m1", Stream: "answer", N: n, Text: text, Replace: true}}
	}
	for name, tc := range map[string]struct {
		snap FeedFrame
	}{
		"the snapshot holds the racing delta": {snapshot(3, "abc")},
		"the snapshot was taken before it":    {snapshot(2, "ab")},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFeeds(16)
			out := f.open(t.Context(), "c1", 0, attaching(func() { f.delta("c1", d(3, "c")) }, tc.snap))
			frames := take(t, out, 1)
			f.delta("c1", d(4, "d"))

			assert.Equal(t, "abcd", apply(append(frames, take(t, out, 2)...)))
		})
	}
}

func TestFeeds_ADeletedChatEndsItsFeedsAfterWhatWasQueued(t *testing.T) {
	f := newFeeds(16)
	attached := make(chan struct{})
	out := f.open(t.Context(), "c1", 0, attaching(func() { close(attached) }))
	<-attached

	f.rows("c1", seqRows(1, 1))
	f.deleted("c1")
	f.rows("c1", seqRows(2, 2))

	frames := drain(out)
	assert.Equal(t, []int64{1}, seqsOf(frames))
	assert.Equal(t, FeedFrame{Type: FeedDeleted}, frames[len(frames)-1])
	assert.Empty(t, f.byChat, "an ended feed leaves the registry")
}

func TestFeeds_AFeedClosesWhenItsContextEndsOrItsAttachFails(t *testing.T) {
	f := newFeeds(16)
	ctx, cancel := context.WithCancel(t.Context())
	out := f.open(ctx, "c1", 0, attaching(nil))
	f.rows("c1", seqRows(1, 1))
	take(t, out, 1)

	cancel()

	assert.Empty(t, drain(out))
	failed := f.open(t.Context(), "c1", 500, func(func(FeedFrame) bool) error { return errors.New("disk gone") })
	assert.Equal(t, []FeedFrame{{Type: FeedFailed}}, drain(failed),
		"a feed that cannot attach fails: asking again at once would fail the same way")
	assert.Empty(t, f.byChat)
}

// knownChats knows one chat, for reads up to its last one when that is set.
type knownChats struct {
	agentchat.EventStore
	id    string
	last  int32
	reads *atomic.Int32
}

func (k knownChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	if id != k.id || k.reads != nil && k.reads.Add(1) > k.last {
		return domain.Chat{}, agentchat.ErrNotFound
	}
	return domain.Chat{ID: id}, nil
}

func marker(t *testing.T, kind string) ledger.Row {
	t.Helper()
	r, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: kind})
	require.NoError(t, err)
	return r
}

func TestOpenFeed_ReplaysAfterSinceThenIsReadyThenLive(t *testing.T) {
	u := newWiringFixture(t)
	u.chats = knownChats{id: "c1"}
	for range 3 {
		_, _, err := u.ledger.Append(t.Context(), "c1", marker(t, ledger.MarkerNotification))
		require.NoError(t, err)
	}

	out, err := u.OpenFeed(t.Context(), "c1", 1)
	require.NoError(t, err)
	frames := take(t, out, 3)
	_, _, err = u.ledger.Append(t.Context(), "c1", marker(t, ledger.MarkerNotification))
	require.NoError(t, err)
	frames = append(frames, take(t, out, 1)...)

	assert.Equal(t, []int64{2, 3, 4}, seqsOf(frames))
	assert.Equal(t, FeedFrame{Type: FeedReady, Seq: 3}, frames[2])
}

// turnRows are turnID's prompt, start, also and end.
func turnRows(t *testing.T, turnID string, also ...ledger.Row) []ledger.Row {
	t.Helper()
	row := func(kind ledger.Kind, body any) ledger.Row {
		r, err := ledger.NewRow(kind, turnID, "", body)
		require.NoError(t, err)
		return r
	}
	rows := []ledger.Row{
		row(ledger.KindUserMessage, ledger.UserMessage{RequestID: turnID, Text: "hi", Origin: ledger.OriginCrowbar}),
		row(ledger.KindTurnStarted, ledger.TurnStarted{RequestID: turnID}),
	}
	return append(append(rows, also...), row(ledger.KindTurnEnded, ledger.TurnEnded{Status: "done"}))
}

// imported writes rows into c1 as its whole ledger.
func imported(t *testing.T, u *Usecase, rows ...ledger.Row) {
	t.Helper()
	for i := range rows {
		rows[i].ChatID, rows[i].Seq, rows[i].At = "c1", int64(i+1), time.Now()
	}
	require.NoError(t, u.ledger.Import(t.Context(), "c1", rows))
}

func TestOpenFeed_ANegativeSinceOpensOnTheNewestTurnsAndSaysWhere(t *testing.T) {
	u := newWiringFixture(t)
	u.chats = knownChats{id: "c1"}
	imported(t, u, slices.Concat(turnRows(t, "T1"), turnRows(t, "T2"), turnRows(t, "T3"))...)

	out, err := u.OpenFeed(t.Context(), "c1", -1)
	require.NoError(t, err)
	frames := take(t, out, 4)

	assert.Equal(t, []int64{7, 8, 9}, seqsOf(frames))
	assert.Equal(t, []int64{6, 0, 0}, []int64{frames[0].From, frames[1].From, frames[2].From})
	assert.Equal(t, FeedFrame{Type: FeedReady, Seq: 9}, frames[3])
}

func TestOpenFeed_AWindowReachesBackToATurnWhoseSubagentStillRuns(t *testing.T) {
	u := newWiringFixture(t)
	u.chats = knownChats{id: "c1"}
	start, err := ledger.NewRow(ledger.KindSubagent, "T1", "S1", ledger.SubagentBody{Phase: ledger.SubagentStart})
	require.NoError(t, err)
	imported(t, u, slices.Concat(turnRows(t, "T1", start), turnRows(t, "T2"))...)

	out, err := u.OpenFeed(t.Context(), "c1", -1)
	require.NoError(t, err)

	assert.Equal(t, []int64{1, 2, 3, 4, 5, 6, 7}, seqsOf(take(t, out, 7)))
}

func TestOpenFeed_AnUnknownChatIsNotFound(t *testing.T) {
	u := newWiringFixture(t)
	u.chats = knownChats{id: "c1"}

	_, err := u.OpenFeed(t.Context(), "nope", 0)

	require.ErrorIs(t, err, problem.ErrNotFound)
}

func TestOpenFeed_AChatPurgedWhileItOpensEndsDeleted(t *testing.T) {
	u := newWiringFixture(t)
	u.chats = knownChats{id: "c1", last: 1, reads: &atomic.Int32{}}

	out, err := u.OpenFeed(t.Context(), "c1", 0)

	require.NoError(t, err)
	assert.Equal(t, []FeedFrame{{Type: FeedDeleted}}, drain(out))
}

func TestOpenFeed_ASinceBeyondTheHeadResyncsFromTheHead(t *testing.T) {
	u := newWiringFixture(t)
	u.chats = knownChats{id: "c1"}
	for range 2 {
		_, _, err := u.ledger.Append(t.Context(), "c1", marker(t, ledger.MarkerNotification))
		require.NoError(t, err)
	}

	out, err := u.OpenFeed(t.Context(), "c1", 5)

	require.NoError(t, err)
	assert.Equal(t, []FeedFrame{{Type: FeedResync, Seq: 2}}, drain(out))
}

func TestReadBlob_ReadsARefOfAKnownChat(t *testing.T) {
	u := newWiringFixture(t)
	u.chats = knownChats{id: "c1"}
	ref, err := u.blobs.Put([]byte(`{"command":"ls"}`))
	require.NoError(t, err)
	unknown := ledgerstore.RefPrefix + strings.Repeat("0", 64)

	got, err := u.ReadBlob(t.Context(), "c1", ref)
	require.NoError(t, err)
	assert.JSONEq(t, `{"command":"ls"}`, string(got))

	_, err = u.ReadBlob(t.Context(), "nope", ref)
	require.ErrorIs(t, err, problem.ErrNotFound, "an unknown chat")
	_, err = u.ReadBlob(t.Context(), "c1", unknown)
	require.ErrorIs(t, err, problem.ErrNotFound, "a ref no blob is stored under")
	_, err = u.ReadBlob(t.Context(), "c1", strings.TrimPrefix(ref, ledgerstore.RefPrefix))
	require.ErrorIs(t, err, problem.ErrInvalidArgument, "a ref rows never carry")
}
