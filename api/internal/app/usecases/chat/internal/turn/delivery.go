package turn

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/dedup"
)

// IngestHookDelivery is the idempotent ingress for one relayed hook: it skips a
// delivery id whose effects already committed (a relay retry), buffers it if the
// runner is still starting, runs its effects, then marks the id done.
//
// The runner gate is held across the WHOLE of that, because what must not
// interleave is the ingestion, not the bookkeeping.
func (t *Turns) IngestHookDelivery(
	ctx context.Context,
	deliveryID, runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) error {
	parsed, err := uuid.Parse(deliveryID)
	if err != nil || parsed.String() != deliveryID {
		return fmt.Errorf("agent: hook delivery id must be a canonical UUID")
	}
	hash := dedup.Hash(runnerID, provider, canonicalEvent, rawPayload)

	defer t.hookGates.Lock(runnerID)()
	done, err := t.hookDeliveries.Done(deliveryID, hash)
	if err != nil || done {
		return err
	}

	if handled, enqueueErr := t.pendingHooks.EnqueueDelivery(
		runnerID, provider, canonicalEvent, rawPayload, deliveryID, hash,
	); handled {
		return enqueueErr
	}

	deliveryCtx := inflight.WithDeliveryID(ctx, deliveryID)
	if err := t.ingestHookNow(deliveryCtx, runnerID, provider, canonicalEvent, rawPayload); err != nil {
		return err
	}
	t.hookDeliveries.Complete(deliveryID, hash)
	return nil
}
