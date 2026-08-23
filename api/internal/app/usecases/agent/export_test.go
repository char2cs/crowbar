package agent

import (
	"context"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
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
const WaitingForTurnLog = waitingForTurnLog

// ComposeContext exposes the {context} composition rule to this package's (external)
// tests. It is not production surface: this file is compiled only under `go test`.
//
// The rule is reached directly rather than through a spawn because one of its inputs
// comes from the process-wide config singleton, and config.Get MEMOISES on first use
// — so a spawn-level test could not present a blanked capabilities_instruction
// without depending on which test in the binary ran first. A composition rule tested
// through a memoised global is a test that passes for the wrong reason.
var ComposeContext = composeContext

// SetPromptJournalDirSync installs a deterministic durability fault for external
// package tests. It is test-only surface; production always uses fsync+close on
// the journal parent directory after the atomic rename.
//
// It REPLACES the journal, so it must be called before the chat under test has
// submitted anything. The prompt journal holds no state between calls, so a
// replacement loses nothing.
func SetPromptJournalDirSync(u RunnerUsecase, syncDir func(string) error) {
	u.(*runnerUsecase).prompts = agentjournal.NewPromptRequests(agentjournal.WithDirSync(syncDir))
}

// RequirePromptRestart exposes the delivery guard SubmitPrompt runs before it
// touches anything. It is test-only surface: this file is compiled only under
// `go test`.
//
// It is exposed because the guard's refusal branch has no descriptor that can
// reach it. A strategy this daemon cannot drive is refused at LOAD by the
// descriptor rules, so the only way to ask the guard about one is to hand it a
// descriptor built in Go — which is the point: the day a strategy is made
// declarable before it is made deliverable, this is the lock that still holds,
// and a lock nothing exercises is a lock nobody notices going missing.
func RequirePromptRestart(
	ctx context.Context,
	u RunnerUsecase,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) error {
	return u.(*runnerUsecase).requirePromptRestart(ctx, chatID, live, descriptor)
}

// SetHookDeliveryDirSync installs a deterministic durability fault for external
// package tests. It is test-only surface; production always uses fsync+close on
// the hook delivery directory after the atomic rename.
//
// It REPLACES the journal, so it must be called before the runner under test
// has delivered anything: the in-memory completion markers do not survive it.
func SetHookDeliveryDirSync(u TurnUsecase, syncDir func(string) error) {
	u.(*turnUsecase).hookDeliveries = agentjournal.NewHookDeliveries(agentjournal.WithDirSync(syncDir))
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
	return slices.Contains(u.(*turnUsecase).hookDeliveries.CompletionMarkers(), deliveryID)
}

// HookDeliveryMarkerCount reports how many in-memory completion markers the
// journal is holding.
func HookDeliveryMarkerCount(u TurnUsecase) int {
	return len(u.(*turnUsecase).hookDeliveries.CompletionMarkers())
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
