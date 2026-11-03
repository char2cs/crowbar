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
func (s *service) healedRows(
	ctx context.Context,
) ([]domain.Chat, error) {
	rows, undecodable, err := s.storage.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 || s.rebuiltAlready(undecodable) {
		return rows, nil
	}
	if err := s.rebuild(ctx); err != nil {
		return nil, err
	}
	rows, undecodable, err = s.storage.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		s.markRebuilt(undecodable)
	}
	return rows, nil
}

func (s *service) rebuiltAlready(undecodable []string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rebuiltFor != nil && *s.rebuiltFor == rowSet(undecodable)
}

func (s *service) markRebuilt(undecodable []string) {
	set := rowSet(undecodable)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rebuiltFor = &set
}

func rowSet(ids []string) string {
	sorted := slices.Sorted(slices.Values(ids))
	return strings.Join(sorted, "\x00")
}

// rebuild enumerates every aggregate id in the event log and Replays each into
// the read model. It is a no-op when the event store cannot enumerate its ids or
// when the log is empty (nothing to Replay), so an empty model over an empty log
// stays empty.
//
// asynx.Replay rebuilds a SINGLE aggregate id, so a lost read model cannot be
// healed by one call: rebuild enumerates every id the event log holds via the
// event store's serialize.AggregateLister capability, then Replays each id back
// into state/store/agent_chat.db.
//
// A single aggregate that fails to Replay — a pre-cutover payload the current
// reducer cannot fold, or any other corrupt entry in the shared event log — is
// logged and SKIPPED, not propagated: every other aggregate must still heal.
// Before this, one bad id emptied the whole read model, because the caller
// (ListChats/GetChat) has nothing to show while rebuild is erroring.
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
