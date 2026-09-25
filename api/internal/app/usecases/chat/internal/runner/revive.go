// Package runner (file revive.go) is the server-owned revive: sending to a
// dormant chat brings its provider back through the resume ladder, in the same
// spawn-gate hold as the delivery. The client only ever sends; it never
// orchestrates a revive of its own.
package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// reviveForDelivery puts a CLI on chatID if it has none. park is the gate's
// park context, so a Stop preempting this revive abandons it cleanly.
func (rs *Runners) reviveForDelivery(ctx, park context.Context, chatID string) error {
	_, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, agentrunner.ErrNotFound) {
		return fmt.Errorf("agent: submit prompt: live runner: %w", err)
	}
	providerID, err := rs.lastActiveProviderID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: revive: %w", err)
	}
	defer rs.enterPhase(ctx, chatID, snapshot.PhaseStarting)()
	if _, err := rs.switchProviderLocked(ctx, park, chatID, providerID); err != nil {
		return fmt.Errorf("agent: submit prompt: revive: %w", err)
	}
	return nil
}
