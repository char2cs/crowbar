package stream

import (
	"context"
	"errors"
	"log/slog"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type Feeds interface {
	OpenFeed(ctx context.Context, chatID string, since, windowBytes int64) (<-chan agentusecase.FeedFrame, error)
}

type Telemetries interface {
	TelemetryOf(ctx context.Context, chatID string) (any, bool)
}

type ChatSource struct {
	feeds     Feeds
	telemetry Telemetries
	encode    Encoder
}

func NewChatSource(feeds Feeds, telemetry Telemetries, encode Encoder) *ChatSource {
	return &ChatSource{feeds: feeds, telemetry: telemetry, encode: encode}
}

func (s *ChatSource) Attach(ctx context.Context, chatID string, since int64, emit func(stream.Frame)) error {
	frames, err := s.feeds.OpenFeed(ctx, chatID, since, stream.WindowBytes(ctx))
	if problem.CodeOf(err) == problem.NotFound {
		slog.DebugContext(ctx, "stream: chat refused", "chat_id", chatID, "err", err)
		return problem.New(problem.NotFound, "That chat no longer exists")
	}
	if err != nil {
		return unreadable(ctx, chatID, err)
	}
	s.emitTelemetry(ctx, chatID, emit)
	for f := range frames {
		if f.Type == agentusecase.FeedFailed {
			return unreadable(ctx, chatID, errors.New("its feed failed"))
		}
		emit(chatFrame(f))
	}
	return nil
}

func (s *ChatSource) emitTelemetry(ctx context.Context, chatID string, emit func(stream.Frame)) {
	report, ok := s.telemetry.TelemetryOf(ctx, chatID)
	if !ok {
		return
	}
	data, err := s.encode(stream.KindTelemetry, report)
	if err != nil {
		slog.ErrorContext(ctx, "stream: encode a chat's telemetry", "chat_id", chatID, "err", err)
		return
	}
	emit(stream.Frame{T: stream.FrameEntity, Kind: string(stream.KindTelemetry), ID: chatID, Data: data})
}

func unreadable(ctx context.Context, chatID string, err error) error {
	slog.WarnContext(ctx, "stream: chat unreadable", "chat_id", chatID, "err", err)
	return problem.New(problem.Unavailable, "This chat's history could not be read")
}

func chatFrame(f agentusecase.FeedFrame) stream.Frame {
	switch {
	case f.Row != nil:
		return stream.Frame{T: stream.FrameRow, Row: f.Row, From: f.From}
	case f.Delta != nil:
		return stream.Frame{T: stream.FrameDelta, Delta: f.Delta}
	case f.Type == agentusecase.FeedReady:
		return stream.Frame{T: stream.FrameSynced, Seq: f.Seq}
	case f.Type == agentusecase.FeedDeleted:
		return stream.Closed("", stream.ReasonDeleted, nil)
	}
	return stream.Frame{T: stream.FrameClosed, Reason: stream.ReasonResync, Seq: f.Seq}
}
