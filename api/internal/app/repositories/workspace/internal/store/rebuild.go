package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventKeyPrefix is the namespace asynx prepends to an aggregate's event key.
// AggregateLister returns raw keys (events: and snapshots:), so rebuild keeps
// only events keys and strips this prefix; asynx.Replay re-prepends it.
const eventKeyPrefix = "events:"

// ListOrRebuild returns the read model, first rebuilding it when it is empty but
// the event log still holds aggregates. It is the per-request path; the boot sweep
// uses the raw List so startup never pays a replay.
func (s *service) ListOrRebuild(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := s.store.List(ctx, s.fromLog)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	if err := s.rebuild(ctx); err != nil {
		return nil, err
	}
	return s.store.List(ctx, s.fromLog)
}

// fromLog answers an undecodable row from its history, as per-id reads do, at
// one fold per such row per list; a history that will not fold either fails
// the list, as nothing is known about that workspace.
func (s *service) fromLog(
	ctx context.Context,
	id string,
) (*domain.Workspace, error) {
	ws, err := s.ax.Get(ctx, id)
	if errors.Is(err, asynxModels.ErrNotFound) || errors.Is(err, asynxModels.ErrForgetInProgress) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("workspace store: row %s will not decode and its history will not fold: %w", id, err)
	}
	return &ws, nil
}

// rebuild Replays every aggregate id the event log lists into the read model
// (asynx.Replay heals one id at a time); without an AggregateLister it is a no-op.
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
		if err := errors.Join(s.replay(ctx, id)); err != nil {
			return fmt.Errorf("workspace store: replay %s: %w", id, err)
		}
	}
	return nil
}

// replay folds id's stream into the read model, returning Replay's error and the
// first save error apart. A Forget marker deletes the row, so a tombstoned
// stream never heals back as a ghost.
func (s *service) replay(
	ctx context.Context,
	id string,
) (replayErr, saveErr error) {
	replayErr = s.ax.Replay(ctx, id, 1, 0, func(ctx context.Context, evt asynxModels.Event[domain.Workspace]) {
		switch {
		case saveErr != nil:
		case evt.EventName == asynxModels.ForgetEventName:
			saveErr = s.store.Drop(ctx, id)
		default:
			saveErr = s.store.Fold(ctx, evt.Aggregate)
		}
	})
	return replayErr, saveErr
}
