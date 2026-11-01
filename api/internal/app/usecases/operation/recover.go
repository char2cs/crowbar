package operation

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// retained is how many settled operations survive a restart.
const retained = 500

// Recover fails what a restart interrupted, then drops settled operations past
// the newest retained. It runs once at boot, before the API opens.
func (s *Service) Recover(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ops, err := s.store.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("operation: recover: %w", err)
	}
	for i := range ops {
		op := ops[i]
		if settled(op.State) {
			continue
		}
		op.State = domain.OpFailed
		op.Error = &problem.Problem{Code: problem.Interrupted, Message: interruptedMessage}
		settleItems(&op)
		if err := s.saveLocked(ctx, &op); err != nil {
			return fmt.Errorf("operation: recover: %w", err)
		}
	}
	return s.prune(ctx)
}

// pruneLocked keeps the cap after a settle; s.mu is held. Nothing returns its
// failure, so it is logged and kept for Shutdown.
func (s *Service) pruneLocked(ctx context.Context) {
	if err := s.prune(ctx); err != nil {
		slog.ErrorContext(ctx, "operation: prune", "err", err)
		s.saveErrs = append(s.saveErrs, err)
	}
}

// prune drops settled operations past the newest retained; s.mu is held.
func (s *Service) prune(ctx context.Context) error {
	ctx = context.WithoutCancel(ctx)
	ops, err := s.store.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("operation: prune: %w", err)
	}
	var done []domain.Operation
	for _, op := range ops {
		if settled(op.State) {
			done = append(done, op)
		}
	}
	if len(done) <= retained {
		return nil
	}
	sort.SliceStable(done, func(i, j int) bool { return done[i].UpdatedAt.After(done[j].UpdatedAt) })
	for _, op := range done[retained:] {
		if err := s.store.Delete(ctx, op.ID); err != nil {
			return fmt.Errorf("operation: prune %s: %w", op.ID, err)
		}
	}
	return nil
}
