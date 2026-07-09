package store

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventKeyPrefix is the namespace asynx prepends to an aggregate id when it stores
// that aggregate's events (its snapshots use "snapshots:"). The crowbar event
// store's AggregateLister returns the RAW store keys — both "events:<id>" and
// "snapshots:<id>" — so rebuild must keep only the events keys and strip this
// prefix to recover the real aggregate id, which asynx.Replay re-prepends itself.
const eventKeyPrefix = "events:"

// ListOrRebuild returns the durable read model, first healing it via a whole-model
// lazy Replay repair when the model is empty but the event log still holds
// aggregates (spec §3.7, decision 7). This is the per-request List path; the boot
// orphan-sweep deliberately uses the raw List (no replay) instead, so startup
// never pays the replay cost.
//
// asynx.Replay rebuilds a SINGLE aggregate id, so a lost List read model cannot be
// healed by one call: rebuild enumerates every id the event log holds via the
// event store's serialize.AggregateLister capability, then Replays each id back
// into state/store/workspace.db. An event store without that capability (or an
// enumeration error) skips the rebuild. A per-id Get never lands here — it folds
// the aggregate directly from the event log.
func (s *service) ListOrRebuild(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	if err := s.rebuild(ctx); err != nil {
		return nil, err
	}
	return s.store.List(ctx)
}

// rebuild enumerates every aggregate id in the event log and Replays each into the
// read model. It is a no-op when the event store cannot enumerate its ids or when
// the log is empty (nothing to Replay), so an empty model over an empty log stays
// empty.
func (s *service) rebuild(
	ctx context.Context,
) error {
	lister, ok := s.es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("workspace store: enumerate aggregate ids: %w", err)
	}
	for _, key := range keys {
		id, ok := strings.CutPrefix(key, eventKeyPrefix)
		if !ok {
			// A non-event key (e.g. a "snapshots:<id>" row for the same aggregate)
			// — skip it; the aggregate is rebuilt from its "events:" key.
			continue
		}
		if err := s.ax.Replay(ctx, id, 1, 0, s.foldReplayed); err != nil {
			return fmt.Errorf("workspace store: replay %s: %w", id, err)
		}
	}
	return nil
}

// foldReplayed persists each replayed aggregate into the read model. Replay folds
// the event stream, so evt.Aggregate carries the aggregate's state at that
// version; the final event leaves the current state durable.
func (s *service) foldReplayed(
	ctx context.Context,
	evt asynxModels.Event[domain.Workspace],
) {
	if err := s.store.Fold(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "workspace store: replay fold", "id", evt.Aggregate.ID, "err", err)
	}
}
