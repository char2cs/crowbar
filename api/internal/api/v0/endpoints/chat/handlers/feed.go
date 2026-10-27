package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// The close codes a feed ends on: the client resyncs from the seq it was
// sent, or drops the chat.
const (
	closeResync  = 4000
	closeDeleted = 4001
)

// FeedOpener opens a chat's row feed.
type FeedOpener interface {
	OpenFeed(ctx context.Context, chatID string, since int64) (<-chan agentusecase.FeedFrame, error)
}

// Feed serves GET /v0/chats/:chatId/feed?since=N.
type Feed struct{ feeds FeedOpener }

// NewFeed builds the feed handler over feeds.
func NewFeed(feeds FeedOpener) *Feed { return &Feed{feeds: feeds} }

// Handle upgrades to the chat's feed: every row after since, a ready frame,
// then what the chat does next. An unknown chat is refused before upgrading.
func (h *Feed) Handle(c *gin.Context) {
	since := int64(0)
	if raw := c.Query("since"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			libs.WriteErr(c, http.StatusBadRequest, "since must be a non-negative integer")
			return
		}
		since = n
	}
	if !websocket.IsWebSocketUpgrade(c.Request) {
		libs.WriteErr(c, http.StatusBadRequest, "the feed is a websocket")
		return
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	frames, err := h.feeds.OpenFeed(ctx, c.Param("chatId"), since)
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, message)
		return
	}
	ws.Stream(c, frames, encodeFeedFrame)
}

func encodeFeedFrame(f agentusecase.FeedFrame) ([]byte, int) {
	data, err := json.Marshal(f)
	if err != nil {
		slog.Warn("chat feed: encode frame", "type", f.Type, "err", err)
		return []byte(`{"type":"resync"}`), closeResync
	}
	switch f.Type {
	case agentusecase.FeedResync:
		return data, closeResync
	case agentusecase.FeedDeleted:
		return data, closeDeleted
	}
	return data, 0
}
