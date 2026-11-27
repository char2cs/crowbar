package chat

import (
	"sync/atomic"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type hubFrames struct {
	feed  *atomic.Pointer[ChatFeed]
	feeds *feeds
}

func (h hubFrames) FeedDelta(chatID string, d ledger.FeedDelta) { h.feeds.delta(chatID, d) }

func (h hubFrames) Telemetry(chatID, workspaceID string, report engineagents.Telemetry) {
	if feed := h.feed.Load(); feed != nil && feed.Telemetry != nil {
		feed.Telemetry(chatID, workspaceID, report)
	}
}

func (h hubFrames) TelemetryGone(chatID string) {
	if feed := h.feed.Load(); feed != nil && feed.TelemetryGone != nil {
		feed.TelemetryGone(chatID)
	}
}
