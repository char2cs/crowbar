// This file holds the asynx-backed AgentRunner repository (EventStore) — the
// sole AgentRunner repository, and the only way anything outside this package
// touches a runner. Mutations dispatch the command layer with
// optimistic-concurrency retry; reads delegate to the store package's two
// projections (live runners, append-only placement history).
package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/engine/agents/runner/internal/commands"
	"github.com/char2cs/crowbar/api/internal/engine/agents/runner/internal/store"
)

// maxOCCAttempts bounds optimistic-concurrency retries on ErrPipelineFailed:
// with no per-aggregate writeMu, concurrent Sends to one runner aggregate can
// version-collide, so the losers retry — Send re-reads the current version each
// attempt, so a retry converges. ErrValidation is NEVER retried; ErrQueueFull is
// surfaced as apperr.ErrUnavailable. Mirrors agentchat's/reviewthread's OCC
// contract.
const maxOCCAttempts = 8

// WatchFunc and RunnerEvent are aliases for the store-layer watch seam, exposed so
// callers wire it without importing the internal store package directly.
//
// The repository ANNOUNCES what happened; it does not decide what the frontend is
// told. That decision lives in usecases/chat/internal/fanout.
type (
	WatchFunc   = store.WatchFunc
	RunnerEvent = store.RunnerEvent
)

// StartInput seeds a freshly-spawned CLI, placed on the chat it was spawned into.
type StartInput struct {
	RunnerID        string
	WorkspaceID     string
	ProviderID      string
	TerminalSession string
	ChatID          string
	// LaunchModel and LaunchEffort are what Crowbar rendered into this process's
	// argv. They are recorded because they cannot be recovered afterwards: the
	// CLIs expose no readable current model, so an unrecorded spawn is a process
	// whose selection nobody can ever name again.
	LaunchModel           string
	LaunchEffort          string
	LaunchPermissionLevel string
	Now                   time.Time
}

// EventStore is the asynx-backed AgentRunner aggregate repository.
//
// The read side answers placement questions ONLY, from two projections:
//   - live runners — a row exists exactly while its CLI runs, so ErrNotFound on
//     LiveRunnerForChat/Get means "dormant", not "broken".
//   - append-only placement history — PlacementsForChat answers "which providers
//     have ever run on this chat".
//
// Nothing here reports liveness as STATE: ask the live model (row exists?) or the
// PTY. ExitedAt is an audit tombstone and is never a liveness check.
type EventStore interface {
	// Start records a freshly-spawned CLI, pointed at the chat it was spawned into.
	Start(
		ctx context.Context,
		in StartInput,
	) (agents.Runner, error)
	// Report records the model/effort the CLI said it resolved to at session
	// start; "" leaves that half as it was (see commands.Report).
	Report(
		ctx context.Context,
		runnerID string,
		model string,
		effort string,
	) (agents.Runner, error)
	// Displace takes a runner OFF its chat, leaving its row — and
	// saying NOTHING about whether the process is alive. It is a PLACEMENT fact, the
	// one kind of fact Crowbar solely owns, and it is what makes "at most one runner
	// per chat" true at every instant instead of eventually: a SIGTERM'd CLI does not
	// die synchronously, so the CLI we are taking off a chat has to be taken off it
	// NOW, not whenever it gets around to falling over. Issued (before the kill) by
	// every path that removes a CLI from a chat, and issued even when that kill fails.
	Displace(
		ctx context.Context,
		runnerID string,
	) (agents.Runner, error)
	// Exit tombstones a runner whose PROCESS has died. It is emitted ONLY because
	// that happened — the terminal engine's exit callback, boot reconciliation
	// asking the PTY, or (for a runner that has no PTY) its api connection's own
	// process dying with nothing replacing it — never from an independent opinion
	// about liveness. The projection
	// DROPS the live row, and that deletion is how the chat goes dormant.
	Exit(
		ctx context.Context,
		runnerID string,
		now time.Time,
	) (agents.Runner, error)
	// Get returns the live runner, or ErrNotFound once it has exited.
	Get(
		ctx context.Context,
		runnerID string,
	) (agents.Runner, error)
	// LiveRunnerForChat returns the runner currently pointed at chatID, or
	// ErrNotFound when the chat is dormant. Row-existence IS the liveness answer.
	LiveRunnerForChat(
		ctx context.Context,
		chatID string,
	) (agents.Runner, error)
	// LiveRunnersForChat returns EVERY live runner placed on chatID, newest start first.
	// A Start onto a chat asks it right after its write commits, so it can retire
	// everyone but itself. Serving paths want LiveRunnerForChat — "who holds this chat".
	LiveRunnersForChat(
		ctx context.Context,
		chatID string,
	) ([]agents.Runner, error)
	// PlacementsForChat returns every provider that has ever been placed on a
	// chat, OLDEST ARRIVAL FIRST, from APPEND-ONLY history — so it answers for
	// runners that exited long ago.
	//
	// An empty result means no CLI has ever been started on this chat, which is a
	// real answer and not a miss — so this returns no ErrNotFound.
	PlacementsForChat(
		ctx context.Context,
		chatID string,
	) ([]agents.ChatPlacement, error)
	// AllLive returns every runner the read model believes is still running — the
	// input to boot reconciliation, which asks the PTY (the sole authority) whether
	// each one really is and Exits the ones that are not. On an idle machine an
	// empty result is the normal, truthful answer.
	AllLive(
		ctx context.Context,
	) ([]agents.Runner, error)
	// ForgetChat drops a chat's placement history — the chat-delete cascade, and
	// the ONLY thing permitted to remove append-only history. It deliberately does
	// NOT delete the chat's live runner row: that row belongs to the PTY's
	// lifecycle, so the cascade terminates the process and lets the row follow.
	// Hand-deleting it here would be a second authority on liveness — the exact
	// mistake this aggregate exists to delete.
	ForgetChat(
		ctx context.Context,
		chatID string,
	) error
}

// eventSourced is the asynx-backed EventStore implementation. There is no
// writeMu: per-aggregate safety comes from asynx shard routing plus (id,version)
// optimistic concurrency and the sendWithOCC retry below.
type eventSourced struct {
	ax    asynx.Asynx[agents.Runner]
	store *store.Store
}

// NewEventSourced builds the asynx-backed AgentRunner EventStore over ax (the
// singleton axAgentRunner) and storeDB (state/store/agent_runner.db). It delegates
// to store.New to register the read-model and hub projections on ax, once, for
// the singleton.
//
// broadcast must not be nil (store.New refuses it): the hub projection calls it
// on every event, so a nil one would panic inside a projection goroutine far from
// whoever built the repository.
func NewEventSourced(
	ax asynx.Asynx[agents.Runner],
	storeDB *gormdb.DB,
	watch WatchFunc,
) (EventStore, error) {
	st, err := store.New(storeDB, ax, watch)
	if err != nil {
		return nil, fmt.Errorf("agentrunner: store: %w", err)
	}
	return &eventSourced{ax: ax, store: st}, nil
}

// sendFunc issues one command attempt against the aggregate.
type sendFunc func(
	ctx context.Context,
	cmd asynxModels.Command[agents.Runner],
) (asynxModels.Event[agents.Runner], error)

// occSend runs send with OCC retry and the terminal error disposition contract
// (mirrors agentchat's occSend):
//
//   - success                → returned immediately.
//   - ErrValidation          → surfaced immediately, NEVER retried (→ 422).
//   - ErrQueueFull           → translated to apperr.ErrUnavailable (→ 503),
//     NEVER retried: a full shard queue is backpressure, not a version race.
//   - ErrPipelineFailed      → retried up to maxOCCAttempts; still failing after
//     the retries is an unrecoverable optimistic-concurrency collision, surfaced
//     as ErrPipelineFailed (→ 409).
//   - any other error        → surfaced as-is.
//
// All classification is via errors.Is, never string compare.
func occSend(
	ctx context.Context,
	send sendFunc,
	cmd asynxModels.Command[agents.Runner],
) (asynxModels.Event[agents.Runner], error) {
	var lastErr error
	for range maxOCCAttempts {
		evt, err := send(ctx, cmd)
		if err == nil {
			return evt, nil
		}
		switch {
		case errors.Is(err, asynxModels.ErrValidation):
			return asynxModels.Event[agents.Runner]{}, err
		case errors.Is(err, asynxModels.ErrQueueFull):
			return asynxModels.Event[agents.Runner]{}, fmt.Errorf("agentrunner: send: %w", apperr.ErrUnavailable)
		case errors.Is(err, asynxModels.ErrPipelineFailed):
			lastErr = err
		default:
			return asynxModels.Event[agents.Runner]{}, err
		}
	}
	return asynxModels.Event[agents.Runner]{}, lastErr
}

// sendWithOCC dispatches cmd to the singleton axAgentRunner with OCC retry.
//
// It uses SendWait, not Send: the command returns only once every projection has
// folded it, so a hook that reads the runner right after (Get → CurrentChatID)
// sees the write that just happened. Runner commands fire on spawn, report,
// displace and exit — never per turn.
func (r *eventSourced) sendWithOCC(
	ctx context.Context,
	cmd asynxModels.Command[agents.Runner],
) (asynxModels.Event[agents.Runner], error) {
	return occSend(ctx, r.ax.SendWait, cmd)
}

func (r *eventSourced) Start(
	ctx context.Context,
	in StartInput,
) (agents.Runner, error) {
	evt, err := r.sendWithOCC(ctx, commands.Start{
		RunnerID:              in.RunnerID,
		WorkspaceID:           in.WorkspaceID,
		ProviderID:            in.ProviderID,
		TerminalSession:       in.TerminalSession,
		ChatID:                in.ChatID,
		LaunchModel:           in.LaunchModel,
		LaunchEffort:          in.LaunchEffort,
		LaunchPermissionLevel: in.LaunchPermissionLevel,
		Now:                   in.Now,
	})
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner: start: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) Report(
	ctx context.Context,
	runnerID string,
	model string,
	effort string,
) (agents.Runner, error) {
	evt, err := r.sendWithOCC(ctx, commands.Report{RunnerID: runnerID, Model: model, Effort: effort})
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner: report: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) Displace(
	ctx context.Context,
	runnerID string,
) (agents.Runner, error) {
	evt, err := r.sendWithOCC(ctx, commands.Displace{RunnerID: runnerID})
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner: displace: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) Exit(
	ctx context.Context,
	runnerID string,
	now time.Time,
) (agents.Runner, error) {
	evt, err := r.sendWithOCC(ctx, commands.Exit{RunnerID: runnerID, Now: now})
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner: exit: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) Get(
	ctx context.Context,
	runnerID string,
) (agents.Runner, error) {
	runner, err := r.store.Get(ctx, runnerID)
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner: get: %w", mapNotFound(err))
	}
	return runner, nil
}

func (r *eventSourced) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (agents.Runner, error) {
	runner, err := r.store.LiveRunnerForChat(ctx, chatID)
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner: live runner for chat: %w", mapNotFound(err))
	}
	return runner, nil
}

func (r *eventSourced) LiveRunnersForChat(
	ctx context.Context,
	chatID string,
) ([]agents.Runner, error) {
	runners, err := r.store.LiveRunnersForChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agentrunner: live runners for chat: %w", err)
	}
	return runners, nil
}

func (r *eventSourced) PlacementsForChat(
	ctx context.Context,
	chatID string,
) ([]agents.ChatPlacement, error) {
	placements, err := r.store.PlacementsForChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agentrunner: placements for chat: %w", err)
	}
	return placements, nil
}

func (r *eventSourced) AllLive(
	ctx context.Context,
) ([]agents.Runner, error) {
	rows, err := r.store.AllLive(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentrunner: all live: %w", err)
	}
	return rows, nil
}

func (r *eventSourced) ForgetChat(
	ctx context.Context,
	chatID string,
) error {
	if err := r.store.ForgetChat(ctx, chatID); err != nil {
		return fmt.Errorf("agentrunner: forget chat: %w", err)
	}
	return nil
}

// mapNotFound bridges the store package's local ErrNotFound sentinel (kept local
// there to avoid an import cycle back into this package) to this package's own
// ErrNotFound, so every EventStore caller sees one sentinel regardless of which
// read path served the miss.
func mapNotFound(
	err error,
) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
