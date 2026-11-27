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
