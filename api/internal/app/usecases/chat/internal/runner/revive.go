// Package runner (file revive.go) is the server-owned revive: sending to a
// dormant chat brings its provider back on the chat's session, in the same
// hold of the chat as the delivery. The client only ever sends; it never
// orchestrates a revive of its own.
package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
)

// reviveForDelivery puts a CLI on chatID if it has none. Its waits park on h,
// so a Stop abandons this revive cleanly.
func (rs *Runners) reviveForDelivery(ctx context.Context, h *hold, chatID string) error {
	_, live, err := rs.settledRunner(h)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: %w", err)
	}
	if live {
		return nil
	}
	providerID, err := rs.lastActiveProviderID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: revive: %w", err)
	}
	defer rs.enterPhase(ctx, chatID, snapshot.PhaseStarting)()
	if _, err := rs.switchProviderLocked(ctx, h, chatID, providerID); err != nil {
		return fmt.Errorf("agent: submit prompt: revive: %w", err)
	}
	return nil
}
