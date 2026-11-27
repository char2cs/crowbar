package stream

import (
	"context"
	"encoding/json"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type EntityKind string

const (
	KindOperation    EntityKind = "operation"
	KindReviewThread EntityKind = "review_thread"
	KindProject      EntityKind = "project"
	KindRepo         EntityKind = "repo"
	KindWorkspace    EntityKind = "workspace"
	KindChatEntity   EntityKind = "chat"
	KindFolder       EntityKind = "folder"
	KindTelemetry    EntityKind = "telemetry"
	KindPresence     EntityKind = "presence"
)

const (
	FrameReady  = "ready"
	FrameEntity = "entity"
	FrameSynced = "synced"
	FrameClosed = "closed"
	FrameSignal = "signal"
	FrameError  = "error"
	FrameRow    = "row"
	FrameDelta  = "delta"
)

const (
	SignalGitStatus   = "git_status"
	SignalFileChange  = "file_change"
	SignalDiagnostics = "diagnostics"
)

const (
	ReasonResync  = "resync"
	ReasonRefused = "refused"
	ReasonDeleted = "deleted"
)

const QueueSize = 1024

const HardQueueSize = 64 * QueueSize

const (
	MaxScopeLen = 256
	MaxScopes   = 256
)

const CloseResync = 4000

type Frame struct {
	T       string            `json:"t"`
	Scope   string            `json:"scope,omitempty"`
	Epoch   string            `json:"epoch,omitempty"`
	Kind    string            `json:"kind,omitempty"`
	ID      string            `json:"id,omitempty"`
	V       uint64            `json:"v,omitempty"`
	Data    json.RawMessage   `json:"data,omitempty"`
	Gone    bool              `json:"gone,omitempty"`
	Seq     int64             `json:"seq,omitempty"`
	From    int64             `json:"from,omitempty"`
	Row     *ledger.Row       `json:"row,omitempty"`
	Delta   *ledger.FeedDelta `json:"delta,omitempty"`
	Reason  string            `json:"reason,omitempty"`
	Problem *problem.Problem  `json:"problem,omitempty"`
	Unread  []string          `json:"unread,omitempty"`
}

func Ready(epoch string) Frame { return Frame{T: FrameReady, Epoch: epoch} }

func Synced(scope string, unread ...string) Frame {
	return Frame{T: FrameSynced, Scope: scope, Unread: unread}
}

func Closed(scope, reason string, p *problem.Problem) Frame {
	return Frame{T: FrameClosed, Scope: scope, Reason: reason, Problem: p}
}

func Errored(p problem.Problem) Frame { return Frame{T: FrameError, Problem: &p} }

const (
	OpSub   = "sub"
	OpUnsub = "unsub"
)

type Request struct {
	Op    string `json:"op"`
	Scope string `json:"scope"`
	Since int64  `json:"since,omitempty"`
	Bytes int64  `json:"bytes,omitempty"`
}

type windowBytesKey struct{}

func WithWindowBytes(ctx context.Context, n int64) context.Context {
	if n <= 0 {
		return ctx
	}
	return context.WithValue(ctx, windowBytesKey{}, n)
}

func WindowBytes(ctx context.Context) int64 {
	n, _ := ctx.Value(windowBytesKey{}).(int64)
	return n
}
