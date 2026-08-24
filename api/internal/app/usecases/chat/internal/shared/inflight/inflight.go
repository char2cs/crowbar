// Package inflight is the process-local state that orders work Crowbar has
// started and has not yet seen finish.
//
// None of it is durable and none of it may be: every value here describes a live
// process — a CLI being started, a turn being answered, a hook arriving before its
// runner row exists. A daemon restart kills every process it could have been
// describing, so an empty inflight after a restart is the truth, not a loss.
//
// It is built ONCE per daemon, in the chat usecase's New, and the same values are
// handed to every component that needs them. A second instance of any of them is a
// silent wedge: two turn registries means a switch parks on a turn nothing will
// ever complete, and two gates means two CLIs on one chat.
package inflight

import (
	"context"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight/internal/gate"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight/internal/pending"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight/internal/turnstate"
)

// Gate serialises the user-initiated spawn paths of one chat.
type Gate = gate.Gate

// NewGate returns an empty per-chat gate.
func NewGate() *Gate { return gate.New() }

// Turns is the registry of turns currently in flight, keyed by runner.
type Turns = turnstate.Turns

// NewTurns returns an empty in-flight-turn registry.
func NewTurns() *Turns { return turnstate.NewTurns() }

// Work mirrors the authoritative Working flag the turn commands return.
type Work = turnstate.Work

// NewWork returns an empty work mirror.
func NewWork() *Work { return turnstate.NewWork() }

// Hooks buffers hooks fired before their runner row exists.
type Hooks = pending.Hooks

// Hook is one buffered hook.
type Hook = pending.Hook

// NewHooks returns an empty fork-before-persistence barrier.
func NewHooks() *Hooks { return pending.New() }

// deliveryKey carries the hook delivery id being ingested down to the recorders
// that need it. There is exactly ONE definition of it, and every consumer reads
// it through DeliveryID.
//
// A second, structurally identical key type would compare unequal, so the turn id
// derived from it would silently fall back to a fresh UUID, the per-row dedupe key
// would stop deduplicating, and a retried delivery would append a duplicate user
// turn and a duplicate assistant message instead of being absorbed. That is why it
// lives here, in the package both the ingest side and the record side already
// depend on, rather than in either of them.
type deliveryKey struct{}

// WithDeliveryID marks ctx as ingesting one relayed hook delivery.
func WithDeliveryID(ctx context.Context, deliveryID string) context.Context {
	return context.WithValue(ctx, deliveryKey{}, deliveryID)
}

// DeliveryID returns the hook delivery ctx is ingesting, or empty for an
// un-journalled ingress (the daemon's own replay, and any forwarder predating the
// relay journal).
func DeliveryID(ctx context.Context) string {
	id, _ := ctx.Value(deliveryKey{}).(string)
	return id
}

// RecordID is the id a durable record ingested under ctx is keyed by: the hook
// delivery id when there is one, and a fresh uuid otherwise.
//
// Keying on the delivery is what makes a relay retry idempotent — the second
// arrival writes the same row id as the first, so the store absorbs it instead of
// appending a duplicate turn. An un-journalled ingress has nothing to be
// idempotent about, so it gets a fresh id.
func RecordID(ctx context.Context) string {
	if id := DeliveryID(ctx); id != "" {
		return id
	}
	return uuid.NewString()
}
