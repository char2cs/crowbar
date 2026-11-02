package stream

import (
	"encoding/json"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// EntityKind names the payload of an entity frame.
type EntityKind string

// The entity kinds a frame carries.
const (
	// KindOperation is a domain.Operation.
	KindOperation EntityKind = "operation"
	// KindReviewThread is one review thread of a workspace.
	KindReviewThread EntityKind = "review_thread"
	// The sidebar's entities, each on the projects scope.
	KindProject   EntityKind = "project"
	KindRepo      EntityKind = "repo"
	KindWorkspace EntityKind = "workspace"
	// KindChatEntity is named apart from the chat scope kind.
	KindChatEntity EntityKind = "chat"
	KindFolder     EntityKind = "folder"
	// KindTelemetry is a chat's newest usage report, on its chat scope.
	KindTelemetry EntityKind = "telemetry"
)

// Frame types, the value of Frame.T.
const (
	FrameReady  = "ready"
	FrameEntity = "entity"
	FrameSynced = "synced"
	FrameClosed = "closed"
	// FrameSignal is news about a scope that has no version: git status, a file change.
	FrameSignal = "signal"
	// FrameError is a refused request that names no scope it may drop.
	FrameError = "error"
	// FrameRow is a chat's ledger row; FrameDelta streamed text no row holds yet.
	FrameRow   = "row"
	FrameDelta = "delta"
)

// The kinds of signal frame.
const (
	SignalGitStatus   = "git_status"
	SignalFileChange  = "file_change"
	SignalDiagnostics = "diagnostics"
)

// Why a scope was closed.
const (
	ReasonResync  = "resync"
	ReasonRefused = "refused"
	ReasonDeleted = "deleted"
)

// QueueSize bounds the live frames waiting on one connection.
const QueueSize = 1024

// HardQueueSize bounds every frame waiting on one connection, snapshots too,
// so a client that stopped reading is closed instead of growing forever.
const HardQueueSize = 64 * QueueSize

// MaxScopeLen and MaxScopes bound what one connection may ask for.
const (
	MaxScopeLen = 256
	MaxScopes   = 256
)

// CloseResync is the WebSocket close code of a connection whose queue overflowed.
const CloseResync = 4000

// Frame is one message to the client, discriminated by T.
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
	Row     *ledger.Row       `json:"row,omitempty"`
	Delta   *ledger.FeedDelta `json:"delta,omitempty"`
	Reason  string            `json:"reason,omitempty"`
	Problem *problem.Problem  `json:"problem,omitempty"`
	// Unread names the parts of a synced scope its snapshot could not read:
	// the client keeps what it holds of them rather than counting them gone.
	Unread []string `json:"unread,omitempty"`
}

// Ready is every connection's first frame: a new epoch voids every version the client holds.
func Ready(epoch string) Frame { return Frame{T: FrameReady, Epoch: epoch} }

// Synced says the scope's snapshot is complete, save for the parts unread names.
func Synced(scope string, unread ...string) Frame {
	return Frame{T: FrameSynced, Scope: scope, Unread: unread}
}

// Closed ends the client's hold on scope.
func Closed(scope, reason string, p *problem.Problem) Frame {
	return Frame{T: FrameClosed, Scope: scope, Reason: reason, Problem: p}
}

// Errored refuses a request as the connection's, not a scope's.
func Errored(p problem.Problem) Frame { return Frame{T: FrameError, Problem: &p} }

// Request ops.
const (
	OpSub   = "sub"
	OpUnsub = "unsub"
)

// Request is one message from the client.
type Request struct {
	Op    string `json:"op"`
	Scope string `json:"scope"`
	Since int64  `json:"since,omitempty"`
}
