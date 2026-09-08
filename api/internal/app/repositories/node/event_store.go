// This file holds the asynx-backed Node repository (EventStore), mirroring
// api/internal/app/repositories/chat/event_store.go almost exactly, narrowed to
// a position-only aggregate. Nothing else in the codebase may touch the nodes
// GORM table directly — every read/write goes through this surface.
package node

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/node/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/node/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// maxOCCAttempts bounds optimistic-concurrency retries on ErrPipelineFailed:
// with no per-aggregate writeMu, concurrent Sends to one node aggregate can
// version-collide, so the losers retry — Send re-reads the current version
// each attempt, so a retry converges. ErrValidation is NEVER retried;
// ErrQueueFull is surfaced as apperr.ErrUnavailable. Mirrors agentchat's OCC
// contract.
const maxOCCAttempts = 8

// WatchFunc and NodeEvent are aliases for the store-layer watch seam, exposed so
// callers wire it without importing the internal store package directly.
//
// The repository ANNOUNCES what happened; it does not decide what the frontend
// is told — mirrors agentchat's own WatchFunc/ChatEvent split.
type (
	WatchFunc = store.WatchFunc
	NodeEvent = store.NodeEvent
)

// EventStore is the asynx-backed Node aggregate repository: mutations dispatch
// the command layer with optimistic-concurrency retry (sendWithOCC), reads
// delegate to the store package's read-model projection. It is the sole Node
// repository — every later task that mints, moves or reaps a sidebar row's
// position sends through this surface.
type EventStore interface {
	// Create mints a Node. It is the ONE node command that blocks until its
	// projections have folded (SendWait, not Send), mirroring agentchat.Create:
	// every later task that mints a row needs the read-your-write barrier this
	// provides — a caller that immediately lists a newly-created row's siblings
	// must not see the read model still missing it.
	Create(
		ctx context.Context,
		id string,
		kind domain.NodeKind,
		parentID string,
		order int,
	) (domain.Node, error)
	// SetOrder writes a node's index within the sibling space it is already in
	// and leaves its parent alone — the write a DENSIFY owes, as against a move.
	// On the ordinary async Send path: a single drag renumbers a whole level, so
	// blocking each of these on its projection would serialise the drag behind
	// the read model. See commands.SetOrder's own doc for the race this split
	// exists to avoid.
	SetOrder(
		ctx context.Context,
		id string,
		order int,
	) error
	// SetPlacement writes where a node sits in the tree: the row it hangs off
	// and its dense index within that sibling space — the one row actually
	// dragged. On the ordinary async Send path, for the same reason SetOrder is.
	SetPlacement(
		ctx context.Context,
		id string,
		parentID string,
		order int,
	) error
	// GetNode returns the node for id from the read-model projection, healing it
	// first if it is empty.
	GetNode(
		ctx context.Context,
		id string,
	) (domain.Node, error)
	// ListByParent returns every node whose ParentID is parentID — the
	// sibling-space read every densify pass needs. Callers must not assume any
	// particular ordering; a caller that needs a stable order densifies on Order
	// itself.
	ListByParent(
		ctx context.Context,
		parentID string,
	) ([]domain.Node, error)
	// Forget purges the node aggregate outright via ax.Forget: its synchronous
	// OnForget drops the read-model row AND the underlying event log is erased,
	// so a subsequent GetNode/ListByParent genuinely reports not found. Mirrors
	// agentchat.Forget/reviewthread.DeleteThread. There is deliberately no
	// separate Delete command with its own Validate/EmitEvent — there is no
	// "next state" to emit for a deletion, and Forget is the asynx-native
	// primitive for exactly this.
	Forget(
		ctx context.Context,
		id string,
	) error
}

// eventSourced is the asynx-backed EventStore implementation. There is no
// writeMu: per-aggregate safety comes from asynx shard routing plus
// (id,version) optimistic concurrency and the sendWithOCC retry below.
type eventSourced struct {
	ax    asynx.Asynx[domain.Node]
	store store.Store
}

// NewEventSourced builds the asynx-backed Node EventStore over ax (the
// singleton axNode), es (the same per-type event log ax wraps, retained so the
// read model can self-heal via whole-model lazy Replay), and storeDB
// (state/store/node.db). It delegates to store.New to register the read-model
// and hub projections on ax, once, for the singleton.
func NewEventSourced(
	ax asynx.Asynx[domain.Node],
	es asynxModels.Store,
	storeDB *gormdb.DB,
	watch WatchFunc,
) (EventStore, error) {
	st, err := store.New(storeDB, es, ax, watch)
	if err != nil {
		return nil, fmt.Errorf("node: store: %w", err)
	}
	return &eventSourced{ax: ax, store: st}, nil
}

// sendFunc issues one command attempt against the aggregate.
type sendFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.Node],
) (asynxModels.Event[domain.Node], error)

// occSend runs send with OCC retry and the terminal error disposition contract
// (mirrors agentchat's occSend):
//
//   - success                → returned immediately.
//   - ErrValidation          → surfaced immediately, NEVER retried (→ 422).
//   - ErrQueueFull           → translated to apperr.ErrUnavailable (→ 503),
//     NEVER retried: a full shard queue is backpressure, not a version race.
//   - ErrPipelineFailed      → retried up to maxOCCAttempts; still failing
//     after the retries is an unrecoverable optimistic-concurrency collision,
//     surfaced as ErrPipelineFailed (→ 409).
//   - any other error        → surfaced as-is.
//
// All classification is via errors.Is, never string compare.
func occSend(
	ctx context.Context,
	send sendFunc,
	cmd asynxModels.Command[domain.Node],
) (asynxModels.Event[domain.Node], error) {
	var lastErr error
	for range maxOCCAttempts {
		evt, err := send(ctx, cmd)
		if err == nil {
			return evt, nil
		}
		switch {
		case errors.Is(err, asynxModels.ErrValidation):
			return asynxModels.Event[domain.Node]{}, err
		case errors.Is(err, asynxModels.ErrQueueFull):
			return asynxModels.Event[domain.Node]{}, fmt.Errorf("node: send: %w", apperr.ErrUnavailable)
		case errors.Is(err, asynxModels.ErrPipelineFailed):
			lastErr = err
		default:
			return asynxModels.Event[domain.Node]{}, err
		}
	}
	return asynxModels.Event[domain.Node]{}, lastErr
}

// sendWithOCC dispatches cmd to the singleton axNode with OCC retry.
func (r *eventSourced) sendWithOCC(
	ctx context.Context,
	cmd asynxModels.Command[domain.Node],
) (asynxModels.Event[domain.Node], error) {
	return occSend(ctx, r.ax.Send, cmd)
}

// Create is deliberately the ONLY command on the SendWait path — see the
// EventStore interface doc.
func (r *eventSourced) Create(
	ctx context.Context,
	id string,
	kind domain.NodeKind,
	parentID string,
	order int,
) (domain.Node, error) {
	evt, err := occSend(ctx, r.ax.SendWait, commands.Create{
		ID: id, Kind: kind, ParentID: parentID, Order: order,
	})
	if err != nil {
		return domain.Node{}, fmt.Errorf("node: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) SetOrder(
	ctx context.Context,
	id string,
	order int,
) error {
	if _, err := r.sendWithOCC(ctx, commands.SetOrder{ID: id, Order: order}); err != nil {
		return fmt.Errorf("node: set order: %w", err)
	}
	return nil
}

func (r *eventSourced) SetPlacement(
	ctx context.Context,
	id string,
	parentID string,
	order int,
) error {
	if _, err := r.sendWithOCC(ctx, commands.SetPlacement{ID: id, ParentID: parentID, Order: order}); err != nil {
		return fmt.Errorf("node: set placement: %w", err)
	}
	return nil
}

// Forget purges the node aggregate via ax.Forget (hard delete), mirroring
// agentchat.Forget. See the EventStore interface doc for details.
func (r *eventSourced) Forget(
	ctx context.Context,
	id string,
) error {
	if err := r.ax.Forget(ctx, id); err != nil {
		return fmt.Errorf("node: forget: %w", err)
	}
	return nil
}

func (r *eventSourced) GetNode(
	ctx context.Context,
	id string,
) (domain.Node, error) {
	n, err := r.store.GetNode(ctx, id)
	if err != nil {
		return domain.Node{}, fmt.Errorf("node: get: %w", mapNotFound(err))
	}
	return n, nil
}

func (r *eventSourced) ListByParent(
	ctx context.Context,
	parentID string,
) ([]domain.Node, error) {
	rows, err := r.store.ListByParent(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("node: list by parent: %w", err)
	}
	return rows, nil
}

// mapNotFound bridges the store package's local ErrNotFound sentinel (kept
// local there to avoid an import cycle back into this package) AND the log-fold
// sentinel asynx raises for an unknown aggregate to this package's own
// ErrNotFound, so every EventStore caller sees one sentinel regardless of which
// of the two read paths served the miss.
func mapNotFound(
	err error,
) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, asynxModels.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
