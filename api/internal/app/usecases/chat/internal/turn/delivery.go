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
// interleave is the ingestion, not the bookkeeping — except where the ingest
// waits on a sibling hook of the same runner (stepOutOfHookGate).
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

	ctx, release := t.holdHookGate(ctx, runnerID)
	defer release()
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

// hookGateHold is one ingest's hold on its runner's hook gate.
type hookGateHold struct {
	gates   *inflight.Gate
	key     string
	release func()
}

type hookGateHoldKey struct{}

// holdHookGate takes runnerID's hook gate and returns a ctx that carries the
// hold, so a wait deeper in the ingest can step out of it (see stepOutOfHookGate).
func (t *Turns) holdHookGate(ctx context.Context, runnerID string) (context.Context, func()) {
	hold := &hookGateHold{gates: t.hookGates, key: runnerID, release: t.hookGates.Lock(runnerID)}
	return context.WithValue(ctx, hookGateHoldKey{}, hold), func() { hold.release() }
}

// stepOutOfHookGate runs wait with ctx's hook gate released and retaken after.
// Only for a wait on something another hook of the SAME runner delivers:
// holding the gate through it could only ever end on the clock.
func stepOutOfHookGate(ctx context.Context, wait func()) {
	hold, ok := ctx.Value(hookGateHoldKey{}).(*hookGateHold)
	if !ok {
		wait()
		return
	}
	hold.release()
	defer func() { hold.release = hold.gates.Lock(hold.key) }()
	wait()
}
