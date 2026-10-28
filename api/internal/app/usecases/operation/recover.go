package operation

import (
	"context"
	"fmt"
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
	var done []domain.Operation
	for i := range ops {
		op := ops[i]
		if !settled(op.State) {
			op.State = domain.OpFailed
			op.Error = &problem.Problem{Code: problem.Interrupted, Message: interruptedMessage}
			if err := s.saveLocked(ctx, &op); err != nil {
				return fmt.Errorf("operation: recover: %w", err)
			}
		}
		done = append(done, op)
	}
	sort.SliceStable(done, func(i, j int) bool { return done[i].UpdatedAt.After(done[j].UpdatedAt) })
	for i := retained; i < len(done); i++ {
		if err := s.store.Delete(ctx, done[i].ID); err != nil {
			return fmt.Errorf("operation: prune %s: %w", done[i].ID, err)
		}
	}
	return nil
}
