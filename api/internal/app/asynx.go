package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/node"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newAsynx builds the shared asynx wiring every aggregate type gets. opts run
// last, right before Build, so a caller layers its aggregate's own config (its
// durable read model, an upcaster) onto the same defaults.
func newAsynx[T any](
	es asynxModels.Store,
	ss asynxModels.SnapshotStore,
	opts ...func(*asynx.Builder[T]),
) (asynx.Asynx[T], error) {
	b := asynx.New[T]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		// Without these handlers asynx swallows a live handler's panic or publish
		// error, and a durable read model's failing attempts, so they are logged.
		WithPanicHandler(func(ctx context.Context, evt asynxModels.Event[T], p any) {
			slog.ErrorContext(ctx, "asynx handler panic",
				"aggregate", evt.AggregateID, "event", evt.EventName, "version", evt.Version, "panic", p)
		}).
		WithPublishErrorHandler(func(ctx context.Context, evt asynxModels.Event[T], err error) {
			slog.ErrorContext(ctx, "asynx publish error",
				"aggregate", evt.AggregateID, "event", evt.EventName, "version", evt.Version, "err", err)
		}).
		WithDeliveryErrorHandler(logDeliveryError[T])
	for _, opt := range opts {
		opt(b)
	}
	return b.Build()
}

// aggregates are the per-type asynx singletons, each built with its durable
// read model as subscriber.
type aggregates struct {
	workspace    asynx.Asynx[domain.Workspace]
	reviewThread asynx.Asynx[domain.ReviewThread]
	agentChat    asynx.Asynx[domain.Chat]
	node         asynx.Asynx[domain.Node]
}

func newAggregates(
	adapters *adapter.Container,
	rm *repositories.ReadModels,
) (*aggregates, error) {
	var (
		ax  aggregates
		err error
	)
	if ax.reviewThread, err = newAsynx(adapters.ReviewThreadES(), adapters.ReviewThreadSS(),
		reviewthread.Subscriber(rm.ReviewThread)); err != nil {
		return nil, fmt.Errorf("app: asynx review thread: %w", err)
	}
	// The upcaster strips the patches of fields domain.Workspace no longer has.
	if ax.workspace, err = newAsynx(adapters.WorkspaceES(), adapters.WorkspaceSS(),
		workspace.RetiredFields,
		workspace.Subscriber(rm.Workspace)); err != nil {
		return nil, fmt.Errorf("app: asynx workspace: %w", err)
	}
	if ax.agentChat, err = newAsynx(adapters.AgentChatES(), adapters.AgentChatSS(),
		agentchat.Subscriber(rm.AgentChat)); err != nil {
		return nil, fmt.Errorf("app: asynx agent chat: %w", err)
	}
	if ax.node, err = newAsynx(adapters.NodeES(), adapters.NodeSS(),
		node.Subscriber(rm.Node)); err != nil {
		return nil, fmt.Errorf("app: asynx node: %w", err)
	}
	return &ax, nil
}

// logDeliveryError reports a failed durable attempt; the one that ran out of
// attempts parks the aggregate until a restart, so it says that, not "retrying".
func logDeliveryError[T any](
	ctx context.Context,
	evt asynxModels.Event[T],
	err error,
	attempt int,
) {
	msg := "asynx durable delivery failed; retrying"
	if errors.Is(err, asynxModels.ErrDeliveryParked) {
		msg = "asynx durable delivery parked until the next restart"
	}
	slog.ErrorContext(ctx, msg,
		"aggregate", evt.AggregateID, "event", evt.EventName, "version", evt.Version,
		"attempt", attempt, "err", err)
}
