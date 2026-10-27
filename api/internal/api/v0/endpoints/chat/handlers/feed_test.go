package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// fakeFeeds opens a feed of the frames it holds, for the one chat it knows.
type fakeFeeds struct {
	frames []agentusecase.FeedFrame
	since  int64
	opened bool
}

func (f *fakeFeeds) OpenFeed(_ context.Context, chatID string, since int64) (<-chan agentusecase.FeedFrame, error) {
	if chatID != "chat-1" {
		return nil, fmt.Errorf("chat feed: %s: %w", chatID, apperr.ErrNotFound)
	}
	f.since, f.opened = since, true
	out := make(chan agentusecase.FeedFrame, len(f.frames))
	for _, fr := range f.frames {
		out <- fr
	}
	return out, nil
}

func feedServer(t *testing.T, feeds handlers.FeedOpener) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/chats/:chatId/feed", handlers.NewFeed(feeds).Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv.URL
}

func getFeed(t *testing.T, url string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode
}

func dialStatus(t *testing.T, url string) int {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(url, "http"), nil)
	require.Error(t, err)
	require.Nil(t, conn)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode
}

func TestFeed_UnknownChatIs404(t *testing.T) {
	base := feedServer(t, &fakeFeeds{})

	assert.Equal(t, http.StatusNotFound, dialStatus(t, base+"/chats/nope/feed"))
}

func TestFeed_APlainGetIsRefusedBeforeAFeedOpens(t *testing.T) {
	feeds := &fakeFeeds{}
	base := feedServer(t, feeds)

	assert.Equal(t, http.StatusBadRequest, getFeed(t, base+"/chats/chat-1/feed"))
	assert.False(t, feeds.opened)
}

func TestFeed_BadSinceIs400(t *testing.T) {
	feeds := &fakeFeeds{}
	base := feedServer(t, feeds)

	for _, since := range []string{"-1", "x", "1.5"} {
		assert.Equal(t, http.StatusBadRequest, getFeed(t, base+"/chats/chat-1/feed?since="+since), since)
	}
	assert.False(t, feeds.opened, "a bad since opens no feed")
}

func TestFeed_EachFrameIsJSONAndAnEndClosesWithItsCode(t *testing.T) {
	for end, code := range map[string]int{agentusecase.FeedResync: 4000, agentusecase.FeedDeleted: 4001} {
		t.Run(end, func(t *testing.T) {
			feeds := &fakeFeeds{frames: []agentusecase.FeedFrame{
				{Type: "ready", Epoch: "e1", Seq: 7}, {Type: end, Seq: 7},
			}}
			url := "ws" + strings.TrimPrefix(feedServer(t, feeds), "http") + "/chats/chat-1/feed?since=5"
			conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
			if resp != nil {
				require.NoError(t, resp.Body.Close())
			}
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, conn.Close()) })

			var got []agentusecase.FeedFrame
			for range 2 {
				var fr agentusecase.FeedFrame
				_, data, err := conn.ReadMessage()
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(data, &fr))
				got = append(got, fr)
			}
			_, _, err = conn.ReadMessage()

			assert.Equal(t, feeds.frames, got)
			assert.Equal(t, int64(5), feeds.since)
			var closed *websocket.CloseError
			require.ErrorAs(t, err, &closed)
			assert.Equal(t, code, closed.Code)
		})
	}
}
