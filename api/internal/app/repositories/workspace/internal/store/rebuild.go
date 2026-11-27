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

const eventKeyPrefix = "events:"

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
			continue
		}
		if err := errors.Join(s.replay(ctx, id)); err != nil {
			return fmt.Errorf("workspace store: replay %s: %w", id, err)
		}
	}
	return nil
}

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
