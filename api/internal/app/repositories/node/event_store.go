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

	"github.com/char2cs/crowbar/api/internal/app/repositories/node/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/node/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// WatchFunc and NodeEvent are aliases for the store-layer watch seam, exposed so
// callers wire it without importing the internal store package directly.
//
// The repository ANNOUNCES what happened; it does not decide what the frontend
// is told — mirrors agentchat's own WatchFunc/ChatEvent split.
type (
	WatchFunc = store.WatchFunc
	NodeEvent = store.NodeEvent
	Option    = store.Option
)

// WithPlacementWatch is told, once the read model holds it, of every node
// created, re-parented or deleted.
func WithPlacementWatch(watch func(nodeID string)) Option {
	return store.WithPlacementWatch(watch)
}

// EventStore is the asynx-backed Node aggregate repository: mutations dispatch
// the command layer (asynx serializes each aggregate's commands), reads
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
	// CreateIdempotent is Create for a CALLER-CHOSEN id a concurrent caller
	// might legitimately choose too — never for a server-random one, where
	// two ids can never coincide by chance and any Create failure is a real
	// one. On the "current != nil" refusal Create's own Validate already
	// gives a genuine collision, this reads the winner back directly from
	// the event store (never the read model, which may still be catching up
	// to that commit) instead of surfacing the loss as an error. See
	// workspace.homeWorkspaceID's own doc for the id-collision design this
	// mirrors, and CreateIdempotent's own implementation for why treating
	// EVERY validation failure this way would be wrong for any OTHER
	// caller's id space.
	CreateIdempotent(
		ctx context.Context,
		id string,
		kind domain.NodeKind,
		parentID string,
		order int,
	) (domain.Node, error)
	// SetOrder writes a node's index within the sibling space it is already in
	// and leaves its parent alone — the write a DENSIFY owes, as against a move.
	// On the SendWait path: a placement answers and broadcasts the decided
	// order, and the client's own re-read right behind it must not serve the
	// pre-write projection (caught live: the dragged row landed, then snapped
	// back). A drag writes a handful of rows; the wait costs nothing visible.
	SetOrder(
		ctx context.Context,
		id string,
		order int,
	) error
	// SetPlacement writes where a node sits in the tree: the row it hangs off
	// and its dense index within that sibling space — the one row actually
	// dragged. On the SendWait path, for the same reason SetOrder is.
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
	// ListAll returns every node in one read: the snapshot a walk over many
	// parents takes instead of one ListByParent (a full decode) per parent.
	ListAll(
		ctx context.Context,
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
// writeMu: asynx serializes each aggregate's commands.
type eventSourced struct {
	ax    asynx.Asynx[domain.Node]
	store store.Store
}

// ReadModel is the read model's write side (state/store/node.db). It is opened before the
// asynx instance so it can be that instance's durable subscriber.
type ReadModel = store.Projection

// OpenReadModel opens the read model over storeDB.
func OpenReadModel(
	storeDB *gormdb.DB,
	opts ...Option,
) (*ReadModel, error) {
	rm, err := store.Open(storeDB, opts...)
	if err != nil {
		return nil, fmt.Errorf("node: read model: %w", err)
	}
	return rm, nil
}

// Subscriber is the builder option that makes rm the instance's durable subscriber.
func Subscriber(
	rm *ReadModel,
) func(*asynx.Builder[domain.Node]) {
	return store.Subscriber(rm)
}

// NewEventSourced builds the repository over ax, which must have been built with
// Subscriber(rm). es is the log ax wraps, kept for the lazy rebuild.
func NewEventSourced(
	ax asynx.Asynx[domain.Node],
	es asynxModels.Store,
	rm *ReadModel,
	watch WatchFunc,
) (EventStore, error) {
	st, err := store.New(rm, es, ax, watch)
	if err != nil {
		return nil, fmt.Errorf("node: store: %w", err)
	}
	return &eventSourced{ax: ax, store: st}, nil
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
	evt, err := r.ax.SendWait(ctx, commands.Create{
		ID: id, Kind: kind, ParentID: parentID, Order: order,
	})
	if err != nil {
		return domain.Node{}, fmt.Errorf("node: create: %w", err)
	}
	return evt.Aggregate, nil
}

// CreateIdempotent — see the EventStore interface's own doc for the id-space
// contract this depends on: id must be one a concurrent caller could
// legitimately choose too, never a server-random one.
func (r *eventSourced) CreateIdempotent(
	ctx context.Context,
	id string,
	kind domain.NodeKind,
	parentID string,
	order int,
) (domain.Node, error) {
	n, err := r.Create(ctx, id, kind, parentID, order)
	if err == nil {
		return n, nil
	}
	if !errors.Is(err, asynxModels.ErrValidation) {
		return domain.Node{}, err
	}
	won, getErr := r.ax.Get(ctx, id)
	if getErr != nil {
		// The winner's commit isn't visible yet even at the event-store
		// layer (Get, not the read model) — genuinely unexpected for a
		// same-process serialized aggregate, so surface the ORIGINAL refusal
		// rather than a getErr that names no cause a caller could act on.
		return domain.Node{}, err
	}
	return won, nil
}

func (r *eventSourced) SetOrder(
	ctx context.Context,
	id string,
	order int,
) error {
	if _, err := r.ax.SendWait(ctx, commands.SetOrder{ID: id, Order: order}); err != nil {
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
	if _, err := r.ax.SendWait(ctx, commands.SetPlacement{ID: id, ParentID: parentID, Order: order}); err != nil {
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

func (r *eventSourced) ListAll(
	ctx context.Context,
) ([]domain.Node, error) {
	rows, err := r.store.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("node: list all: %w", err)
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
