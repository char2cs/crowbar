// Package runner (file revive.go) is the server-owned revive: sending to a
// dormant chat brings its provider back on the chat's session, in the same
// spawn-gate hold as the delivery. The client only ever sends; it never
// orchestrates a revive of its own.
package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
)

// reviveForDelivery puts a CLI on chatID if it has none. park is the gate's
// park context, so a Stop preempting this revive abandons it cleanly.
func (rs *Runners) reviveForDelivery(ctx, park context.Context, chatID string) error {
	_, live, err := rs.awaitDormant(park, chatID)
	if err != nil {
		return parkErr(park, fmt.Errorf("agent: submit prompt: %w", err))
	}
	if live {
		return nil
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
