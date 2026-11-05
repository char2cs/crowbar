package store

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// healedRows rebuilds a model with nothing to read, which also rewrites rows
// that will not decode; a rebuild that heals nothing is not repeated until the
// row set changes, as each one replays the whole log.
func (s *service) healedRows(ctx context.Context) ([]domain.Chat, error) {
	rows, undecodable, err := s.storage.FindAll(ctx)
	if err != nil || len(rows) > 0 {
		return rows, err
	}
	set := strings.Join(slices.Sorted(slices.Values(undecodable)), "\x00")
	if last := s.rebuiltFor.Load(); last != nil && *last == set {
		return rows, nil
	}
	if err := s.rebuild(ctx); err != nil {
		return nil, err
	}
	if rows, undecodable, err = s.storage.FindAll(ctx); err == nil && len(rows) == 0 {
		set = strings.Join(slices.Sorted(slices.Values(undecodable)), "\x00")
		s.rebuiltFor.Store(&set)
	}
	return rows, err
}

// rebuild Replays every aggregate id the event log lists into the read model
// (asynx.Replay heals one id at a time); without an AggregateLister it is a no-op.
// An id that fails to Replay is logged and skipped so one bad entry cannot empty the model.
func (s *service) rebuild(
	ctx context.Context,
) error {
	lister, ok := s.es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("agentchat store: enumerate aggregate ids: %w", err)
	}
	for _, key := range keys {
		id, ok := strings.CutPrefix(key, eventKeyPrefix)
		if !ok {
			// A non-event key (e.g. a "snapshots:<id>" row for the same aggregate)
			// — skip it; the aggregate is rebuilt from its "events:" key.
			continue
		}
		replayErr, saveErr := s.replay(ctx, id)
		if saveErr != nil {
			return fmt.Errorf("agentchat store: heal %s: %w", id, saveErr)
		}
		if replayErr != nil {
			slog.ErrorContext(ctx, "agentchat store: replay skipped unreplayable aggregate",
				"id", id, "err", replayErr)
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
	replayErr = s.ax.Replay(ctx, id, 1, 0, func(ctx context.Context, evt asynxModels.Event[domain.Chat]) {
		switch {
		case saveErr != nil:
		case evt.EventName == asynxModels.ForgetEventName:
			saveErr = s.storage.Delete(ctx, id)
		default:
			saveErr = s.storage.Save(ctx, evt.Aggregate)
		}
	})
	return replayErr, saveErr
}
