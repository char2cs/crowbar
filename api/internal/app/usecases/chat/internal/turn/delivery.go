package turn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// IngestHookDelivery is the exactly-once ingress for one relayed hook: it
// dedupes the delivery, buffers it if the runner is still starting, runs its
// effects, then durably records the completion.
//
// The runner gate is held across the WHOLE of that — not inside the journal —
// because what must not interleave is the ingestion, not the record write.
func (t *Turns) IngestHookDelivery(
	ctx context.Context,
	workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) error {
	parsed, err := uuid.Parse(deliveryID)
	if err != nil || parsed.String() != deliveryID {
		return fmt.Errorf("agent: hook delivery id must be a canonical UUID")
	}
	workspaceID, known, err := t.hookDeliveryScope(ctx, workspaceID, runnerID)
	if err != nil || !known {
		return err
	}
	chatsDir, err := t.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("agent: hook delivery: chats dir: %w", err)
	}
	dir := t.hookDeliveries.Dir(chatsDir, runnerID)
	hash := agentjournal.HookDeliveryHash(runnerID, provider, canonicalEvent, rawPayload)

	defer t.hookGates.Lock(runnerID)()
	done, err := t.hookDeliveries.Begin(dir, deliveryID, hash, time.Now())
	if err != nil || done {
		return err
	}

	if handled, enqueueErr := t.pendingHooks.EnqueueDelivery(
		runnerID, provider, canonicalEvent, rawPayload, deliveryID, dir, hash,
	); handled {
		return enqueueErr
	}

	deliveryCtx := inflight.WithDeliveryID(ctx, deliveryID)
	if err := t.ingestHookNow(deliveryCtx, runnerID, provider, canonicalEvent, rawPayload); err != nil {
		return err
	}
	if err := t.hookDeliveries.Complete(dir, deliveryID, hash, time.Now()); err != nil {
		slog.ErrorContext(ctx, "agent: persist completed hook delivery (effects already committed)",
			"runner_id", runnerID, "delivery_id", deliveryID, "err", err)
	}
	return nil
}

func (t *Turns) hookDeliveryScope(
	ctx context.Context,
	workspaceID, runnerID string,
) (string, bool, error) {
	if workspaceID != "" {
		return workspaceID, true, nil
	}
	runner, err := t.runnerStore.Get(ctx, runnerID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("agent: hook delivery: runner: %w", err)
	}
	return runner.WorkspaceID, true, nil
}
