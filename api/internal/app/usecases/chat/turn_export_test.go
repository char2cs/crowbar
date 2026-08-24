package chat

import (
	"context"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
)

// WaitingForTurnLog is the log record a provider switch emits at the INSTANT it parks on
// an in-flight turn, exposed to this package's (external) tests. It is not production
// surface: this file is compiled only under `go test`.
//
// It exists so a test can block on THE SWITCH BEING PARKED — a real, causal signal —
// instead of sleeping and hoping. That matters more here than anywhere else in the
// package: the property under test is a NEGATIVE ("the outgoing CLI is not killed while
// the turn is still running"), and a negative can only be proven against a moment the
// test knows the switch has actually reached.
const WaitingForTurnLog = turn.WaitingForTurnLog

// SetHookDeliveryDirSync installs a deterministic durability fault for external
// package tests. It is test-only surface; production always uses fsync+close on
// the hook delivery directory after the atomic rename.
//
// It REPLACES the journal, so it must be called before the runner under test
// has delivered anything: the in-memory completion markers do not survive it.
func SetHookDeliveryDirSync(u TurnUsecase, syncDir func(string) error) {
	u.(*Usecase).turns.SetHookDeliveries(agentjournal.NewHookDeliveries(agentjournal.WithDirSync(syncDir)))
}

// HookDeliveryCompletedMax is the FIFO cap on the hook delivery journal's
// in-memory completion markers, exposed so an external test can drive past it.
const HookDeliveryCompletedMax = agentjournal.HookDeliveryCompletedMax

// HookDeliveryJournalMax is the cap on records kept in one runner's on-disk hook
// delivery directory, exposed so an external test can drive past it.
const HookDeliveryJournalMax = agentjournal.HookDeliveryJournalMax

// HookDeliveryPruneEvery is how many completions apart the amortised on-disk
// prune runs, exposed so an external test can land its last delivery on a tick.
const HookDeliveryPruneEvery = agentjournal.HookDeliveryPruneEvery

// HookDeliveryJournalMaxAge is the silence after which a whole runner directory
// is reaped, exposed so an external test can backdate a directory past it.
const HookDeliveryJournalMaxAge = agentjournal.HookDeliveryJournalMaxAge

// HookDeliveryDirName is the per-workspace root the runner directories live
// under, exposed so an external test can count what is on disk.
const HookDeliveryDirName = agentjournal.HookDeliveriesDirName

// HookDeliveryMarked reports whether the journal holds an in-memory completion
// marker for deliveryID. It is the discriminator a replay test needs: a marker
// answers begin() without ever reading the disk, so a test that means to
// exercise the on-disk record must first prove the marker is absent.
func HookDeliveryMarked(u TurnUsecase, deliveryID string) bool {
	return slices.Contains(u.(*Usecase).turns.HookDeliveryMarkers(), deliveryID)
}

// HookDeliveryMarkerCount reports how many in-memory completion markers the
// journal is holding.
func HookDeliveryMarkerCount(u TurnUsecase) int {
	return len(u.(*Usecase).turns.HookDeliveryMarkers())
}

// PlantPendingHookDelivery writes an in-flight hook delivery record straight into
// a runner's journal directory. It is test-only surface: production only reaches
// that state by crashing between begin() and complete(), which a test cannot
// stage from the outside — and "an in-flight record is never pruned" is the one
// place the delivery journal's retention policy is the inverse of the prompt
// journal's, so it needs a test that can put in-flight records on disk.
func PlantPendingHookDelivery(
	dir string,
	deliveryID string,
	now time.Time,
) error {
	_, err := agentjournal.NewHookDeliveries().Begin(dir, deliveryID, "", now)
	return err
}

func CloseStalledTurn(u TurnUsecase, ctx context.Context, stall seam.Stall) {
	u.(*Usecase).turns.CloseStalledTurn(ctx, stall)
}
