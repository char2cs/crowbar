package stream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// Feeds opens a chat's row feed.
type Feeds interface {
	OpenFeed(ctx context.Context, chatID string, since int64) (<-chan agentusecase.FeedFrame, error)
}

// ChatSource serves the chat scope: the chat's ledger after since, then what
// it records next. Wire it as Following.
type ChatSource struct{ feeds Feeds }

// NewChatSource builds the chat scope's source over feeds.
func NewChatSource(feeds Feeds) *ChatSource { return &ChatSource{feeds: feeds} }

// Attach follows chatID's feed until it ends or ctx does; an unknown chat is not found.
func (s *ChatSource) Attach(ctx context.Context, chatID string, since int64, emit func(stream.Frame)) error {
	frames, err := s.feeds.OpenFeed(ctx, chatID, since)
	if problem.CodeOf(err) == problem.NotFound {
		slog.DebugContext(ctx, "stream: chat refused", "chat_id", chatID, "err", err)
		return problem.New(problem.NotFound, "That chat no longer exists")
	}
	if err != nil {
		return fmt.Errorf("stream: chat %s: %w", chatID, err)
	}
	for f := range frames {
		if f.Type == agentusecase.FeedFailed {
			return fmt.Errorf("stream: chat %s: %w", chatID, errFeedFailed)
		}
		emit(chatFrame(f))
	}
	return nil
}

// errFeedFailed is a ledger read that failed: the hub closes the connection so
// the client's reconnect backoff paces the retries.
var errFeedFailed = errors.New("the chat's ledger could not be read")

func chatFrame(f agentusecase.FeedFrame) stream.Frame {
	switch {
	case f.Row != nil:
		return stream.Frame{T: stream.FrameRow, Row: f.Row}
	case f.Delta != nil:
		return stream.Frame{T: stream.FrameDelta, Delta: f.Delta}
	case f.Type == agentusecase.FeedReady:
		return stream.Frame{T: stream.FrameSynced, Seq: f.Seq}
	case f.Type == agentusecase.FeedDeleted:
		return stream.Closed("", stream.ReasonDeleted, nil)
	}
	// A resync names the last row sent, so the client tells a reset ledger apart.
	return stream.Frame{T: stream.FrameClosed, Reason: stream.ReasonResync, Seq: f.Seq}
}
