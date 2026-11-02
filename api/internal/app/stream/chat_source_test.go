package stream_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// feeds is a fake chat usecase whose feed sends frames and ends.
type feeds struct {
	frames []agentusecase.FeedFrame
	err    error
	opened []string
}

func (f *feeds) OpenFeed(_ context.Context, chatID string, since int64) (<-chan agentusecase.FeedFrame, error) {
	f.opened = append(f.opened, fmt.Sprintf("%s since %d", chatID, since))
	if f.err != nil {
		return nil, f.err
	}
	out := make(chan agentusecase.FeedFrame, len(f.frames))
	for _, fr := range f.frames {
		out <- fr
	}
	close(out)
	return out, nil
}

// telemetries is a fake report reader: a chat it holds has a report to show.
type telemetries map[string]string

func (t telemetries) TelemetryOf(_ context.Context, chatID string) (any, bool) {
	report, ok := t[chatID]
	return report, ok
}

// The gauge paints from the scope itself: no GET races the feed.
func TestChatSource_EmitsTheChatsTelemetryBeforeItsRows(t *testing.T) {
	row := ledger.Row{ChatID: "c1", Seq: 1}
	src := &feeds{frames: []agentusecase.FeedFrame{{Type: "row", Row: &row}, {Type: agentusecase.FeedReady, Seq: 1}}}
	var got []stream.Frame

	err := appstream.NewChatSource(src, telemetries{"c1": "report"}, encodeJSON).
		Attach(t.Context(), "c1", 0, func(f stream.Frame) { got = append(got, f) })

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, stream.Frame{T: stream.FrameEntity, Kind: string(stream.KindTelemetry), ID: "c1", Data: []byte(`"report"`)}, got[0])
	assert.Equal(t, stream.FrameRow, got[1].T)
}

// openFeeds is a fake whose feed stays open until the test ends.
type openFeeds struct{ done <-chan struct{} }

func (f openFeeds) OpenFeed(context.Context, string, int64) (<-chan agentusecase.FeedFrame, error) {
	out := make(chan agentusecase.FeedFrame)
	go func() {
		<-f.done
		close(out)
	}()
	return out, nil
}

// A live report published while the feed is held wins over the one read at attach.
func TestChatSource_ALiveReportOutranksTheAttachedOne(t *testing.T) {
	feed := openFeeds{done: t.Context().Done()}
	hub := newHub(map[stream.ScopeKind]appstream.Source{
		stream.KindChat: appstream.Following(appstream.NewChatSource(feed, telemetries{"c1": "old"}, encodeJSON)),
	}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "chat:c1")
	first := conn.next(t)
	live := hub.Entity(stream.Scope{Kind: stream.KindChat, Key: "c1"}, stream.KindTelemetry, "c1", "new", 0)
	second := conn.until(t, func(f stream.Frame) bool { return f.T == stream.FrameEntity })

	assert.Equal(t, `"old"`, string(first.Data))
	assert.Less(t, first.V, live, "the attached report is older than any live one")
	assert.Equal(t, `"new"`, string(second[len(second)-1].Data))
}

func TestChatSource_MapsEveryFeedFrame(t *testing.T) {
	row := ledger.Row{ChatID: "c1", Seq: 4}
	delta := ledger.FeedDelta{ID: "m1", Stream: "answer", N: 1, Text: "a"}
	src := &feeds{frames: []agentusecase.FeedFrame{
		{Type: "row", Row: &row},
		{Type: agentusecase.FeedReady, Seq: 4},
		{Type: "delta", Delta: &delta},
		{Type: agentusecase.FeedResync, Seq: 3},
		{Type: agentusecase.FeedDeleted},
	}}
	var got []stream.Frame

	err := appstream.NewChatSource(src, telemetries{}, encodeJSON).Attach(t.Context(), "c1", 2, func(f stream.Frame) { got = append(got, f) })

	require.NoError(t, err)
	assert.Equal(t, []string{"c1 since 2"}, src.opened)
	assert.Equal(t, []stream.Frame{
		{T: stream.FrameRow, Row: &row},
		{T: stream.FrameSynced, Seq: 4},
		{T: stream.FrameDelta, Delta: &delta},
		{T: stream.FrameClosed, Reason: stream.ReasonResync, Seq: 3},
		{T: stream.FrameClosed, Reason: stream.ReasonDeleted},
	}, got)
}

func TestChatSource_AnUnknownChatIsRefused(t *testing.T) {
	src := &feeds{err: fmt.Errorf("chat feed: c9: %w", problem.ErrNotFound)}
	hub := newHub(map[stream.ScopeKind]appstream.Source{
		stream.KindChat: appstream.Following(appstream.NewChatSource(src, telemetries{}, encodeJSON)),
	}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "chat:c9")
	closed := conn.next(t)

	assert.Equal(t, "chat:c9", closed.Scope)
	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	assert.Equal(t, problem.Problem{Code: problem.NotFound, Message: "That chat no longer exists"}, *closed.Problem,
		"the client is told what happened, not the daemon's error chain")
}

// A chat whose ledger cannot be read is refused on its own: asking again at
// once would fail the same way, and the connection's other scopes go on.
func TestChatSource_AFailedFeedRefusesItsScopeOnly(t *testing.T) {
	for name, src := range map[string]*feeds{
		"a failed read": {frames: []agentusecase.FeedFrame{{Type: agentusecase.FeedFailed}}},
		"a failed open": {err: errors.New("chat feed: c1: disk gone")},
	} {
		t.Run(name, func(t *testing.T) {
			hub := newHub(map[stream.ScopeKind]appstream.Source{
				stream.KindChat:     appstream.Following(appstream.NewChatSource(src, telemetries{}, encodeJSON)),
				stream.KindProjects: newEntities(map[string]string{}),
			}, nil)
			conn := newFakeConn()
			served(t, hub, conn)

			conn.send(t, stream.OpSub, "chat:c1")
			closed := conn.next(t)
			conn.send(t, stream.OpSub, "projects")
			conn.until(t, syncedOn("projects"))

			assert.Equal(t, stream.ReasonRefused, closed.Reason)
			assert.Equal(t, problem.Problem{Code: problem.Unavailable, Message: "This chat's history could not be read"},
				*closed.Problem)
			assert.Equal(t, 0, conn.closeCode(), "one broken chat leaves the connection open")
		})
	}
}
