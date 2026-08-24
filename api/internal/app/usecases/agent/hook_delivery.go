package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// hookDeliveryContextKey carries the delivery id being ingested down to the
// recorders that need it. There is exactly ONE definition of it, and every
// consumer reads it through hookDeliveryID.
//
// A second, structurally identical key type would compare unequal, so
// turnID(ctx) would silently fall back to a fresh UUID, rowKey(chatID, turnID)
// would stop deduplicating, and a retried delivery would append a duplicate
// user turn and a duplicate assistant message instead of being absorbed.
type hookDeliveryContextKey struct{}

func hookDeliveryID(ctx context.Context) string {
	id, _ := ctx.Value(hookDeliveryContextKey{}).(string)
	return id
}

// IngestHookDelivery is the exactly-once ingress for one relayed hook: it
// dedupes the delivery, buffers it if the runner is still starting, runs its
// effects, then durably records the completion.
//
// The runner gate is held across the WHOLE of that — not inside the journal —
// because what must not interleave is the ingestion, not the record write.
func (u *turnUsecase) IngestHookDelivery(
	ctx context.Context,
	workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) error {
	parsed, err := uuid.Parse(deliveryID)
	if err != nil || parsed.String() != deliveryID {
		return fmt.Errorf("agent: hook delivery id must be a canonical UUID")
	}
	workspaceID, known, err := u.hookDeliveryScope(ctx, workspaceID, runnerID)
	if err != nil || !known {
		return err
	}
	chatsDir, err := u.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("agent: hook delivery: chats dir: %w", err)
	}
	dir := u.hookDeliveries.Dir(chatsDir, runnerID)
	hash := agentjournal.HookDeliveryHash(runnerID, provider, canonicalEvent, rawPayload)

	defer u.hookGates.lock(runnerID)()
	done, err := u.hookDeliveries.Begin(dir, deliveryID, hash, time.Now())
	if err != nil || done {
		return err
	}

	if handled, enqueueErr := u.pendingHooks.enqueueDelivery(
		runnerID, provider, canonicalEvent, rawPayload, deliveryID, dir, hash,
	); handled {
		return enqueueErr
	}

	deliveryCtx := context.WithValue(ctx, hookDeliveryContextKey{}, deliveryID)
	if err := u.ingestHookNow(deliveryCtx, runnerID, provider, canonicalEvent, rawPayload); err != nil {
		return err
	}
	if err := u.hookDeliveries.Complete(dir, deliveryID, hash, time.Now()); err != nil {
		slog.ErrorContext(ctx, "agent: persist completed hook delivery (effects already committed)",
			"runner_id", runnerID, "delivery_id", deliveryID, "err", err)
	}
	return nil
}

func (u *turnUsecase) hookDeliveryScope(
	ctx context.Context,
	workspaceID, runnerID string,
) (string, bool, error) {
	if workspaceID != "" {
		return workspaceID, true, nil
	}
	runner, err := u.runners.Get(ctx, runnerID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("agent: hook delivery: runner: %w", err)
	}
	return runner.WorkspaceID, true, nil
}
