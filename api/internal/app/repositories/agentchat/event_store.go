// This file holds the asynx-backed AgentChat repository (EventStore) — the sole
// AgentChat repository since the Task 10 cutover retired the bespoke gorm-backed
// Store/gormStore/New. The agent usecase sends every mutation through it.
package agentchat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// maxOCCAttempts bounds optimistic-concurrency retries on ErrPipelineFailed:
// with no per-aggregate writeMu, concurrent Sends to one chat aggregate can
// version-collide, so the losers retry — Send re-reads the current version
// each attempt, so a retry converges. ErrValidation is NEVER retried;
// ErrQueueFull is surfaced as apperr.ErrUnavailable. Mirrors reviewthread's OCC
// contract (spec decision 10).
const maxOCCAttempts = 8

// BroadcastFunc is an alias for the store-layer broadcast type, exposed so
// callers can wire the hub fan-out without importing the internal store
// package directly.
type BroadcastFunc = store.BroadcastFunc

// CreateInput seeds a new AgentChat: identity, workspace, clock. It carries no
// segment/provider/terminal because the chat does not own the CLI talking to it
// — that is the runner (agentrunner.Start), a separate aggregate.
type CreateInput struct {
	ID          string
	WorkspaceID string
	Now         time.Time
}

// EventStore is the asynx-backed AgentChat aggregate repository: mutations
// dispatch the command layer with optimistic-concurrency retry (sendWithOCC),
// reads delegate to the store package's read-model projection. It is the sole
// AgentChat repository — the agent usecase sends every mutation through it.
//
// There is no OpenSegment/EndSegment/BindSession any more, and their absence is
// the point: a process moving between chats used to be an EndSegment on the chat
// it left plus an OpenSegment on the chat it entered — two writes across two
// aggregates with no transaction, which tore in half and bricked a chat. The
// runner aggregate now owns that move as ONE write, and the chat is never written
// to when a CLI leaves it.
type EventStore interface {
	// Create mints a chat. It is the ONE agentchat command that blocks until its
	// projections have folded (SendWait, not Send): the very next thing that
	// happens after a chat is minted is a hook writing into it (a /clear mints a
	// chat and the CLI's first prompt lands milliseconds later), and that hook path
	// reads the chat's existence from the read model before it appends. A Send
	// would let the read model lag behind the mint and the turn would be dropped
	// as "no such chat". No other command needs this: every other write validates
	// against the EVENT LOG (which is synchronous), not the read model.
	Create(
		ctx context.Context,
		in CreateInput,
	) (domain.AgentChat, error)
	StartTurn(
		ctx context.Context,
		chatID string,
		now time.Time,
	) (domain.AgentChat, error)
	// StopTurn closes the turn, recording the level of async work the CLI reported
	// still OUTSTANDING as it went quiet (asyncWork — see domain.AgentChat.AsyncWork).
	// It does NOT assert that work is over: a chat whose CLI ended its turn to wait on
	// a background task keeps Working through the wait. Pass 0 for a provider that
	// reports no such level, which folds Working back to the turn alone.
	StopTurn(
		ctx context.Context,
		chatID string,
		now time.Time,
		asyncWork int,
	) (domain.AgentChat, error)
	// AbandonTurn closes the turn AND zeroes the async-work level. It is the reconcile
	// door — a dead CLI, a displaced runner — and nothing but a reconcile may call it:
	// it is precisely what an ordinary StopTurn must not do.
	//
	// It is what keeps a killed CLI from stranding the spinner forever. The level is
	// only ever restated by the CLI's own next turn_stop, so a CLI that dies with work
	// outstanding leaves its last report standing with nobody left to correct it — and
	// in an event-sourced aggregate that outlives the restart too.
	AbandonTurn(
		ctx context.Context,
		chatID string,
		now time.Time,
	) (domain.AgentChat, error)
	SetTitle(
		ctx context.Context,
		chatID string,
		title string,
		source string,
	) (domain.AgentChat, error)
	// Forget purges the chat aggregate outright via ax.Forget: its synchronous
	// OnForget drops the read-model row AND the underlying event log is
	// erased, so a subsequent GetChat/ListByWorkspace genuinely reports not
	// found. Two callers: the workspace-delete cascade
	// (repositories.Container.forgetAgentChats, Task 12), when the owning
	// workspace itself is gone and the chat has nowhere left to live; and the
	// standalone hard delete (agent.Usecase.PurgeChat, Task 5), which reaps a
	// single chat on user request. Mirrors reviewthread's DeleteThread.
	Forget(
		ctx context.Context,
		id string,
	) error
	GetChat(
		ctx context.Context,
		id string,
	) (domain.AgentChat, error)
	ListChats(
		ctx context.Context,
	) ([]domain.AgentChat, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.AgentChat, error)
}

// eventSourced is the asynx-backed EventStore implementation. There is no
// writeMu: per-aggregate safety comes from asynx shard routing plus
// (id,version) optimistic concurrency and the sendWithOCC retry below.
type eventSourced struct {
	ax    asynx.Asynx[domain.AgentChat]
	store store.Store
}

// NewEventSourced builds the asynx-backed AgentChat EventStore over ax (the
// singleton axAgentChat), es (the same per-type event log ax wraps, retained
// so the read model can self-heal via whole-model lazy Replay), and storeDB
// (state/store/agent_chat.db). It delegates to store.New to register the
// read-model and hub projections on ax, once, for the singleton.
func NewEventSourced(
	ax asynx.Asynx[domain.AgentChat],
	es asynxModels.Store,
	storeDB *gormdb.DB,
	broadcast BroadcastFunc,
) (EventStore, error) {
	st, err := store.New(storeDB, es, ax, broadcast)
	if err != nil {
		return nil, fmt.Errorf("agentchat: store: %w", err)
	}
	return &eventSourced{ax: ax, store: st}, nil
}

// sendFunc issues one command attempt against the aggregate.
type sendFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.AgentChat],
) (asynxModels.Event[domain.AgentChat], error)

// occSend runs send with OCC retry and the terminal error disposition
// contract (mirrors reviewthread's occSend):
//
//   - success                → returned immediately.
//   - ErrValidation          → surfaced immediately, NEVER retried (→ 422).
//   - ErrQueueFull           → translated to apperr.ErrUnavailable (→ 503),
//     NEVER retried: a full shard queue is backpressure, not a version race.
//   - ErrPipelineFailed      → retried up to maxOCCAttempts; still failing
//     after the retries is an unrecoverable optimistic-concurrency collision,
//     surfaced as ErrPipelineFailed (→ 409).
//   - any other error        → surfaced as-is.
//
// All classification is via errors.Is, never string compare.
func occSend(
	ctx context.Context,
	send sendFunc,
	cmd asynxModels.Command[domain.AgentChat],
) (asynxModels.Event[domain.AgentChat], error) {
	var lastErr error
	for range maxOCCAttempts {
		evt, err := send(ctx, cmd)
		if err == nil {
			return evt, nil
		}
		switch {
		case errors.Is(err, asynxModels.ErrValidation):
			return asynxModels.Event[domain.AgentChat]{}, err
		case errors.Is(err, asynxModels.ErrQueueFull):
			return asynxModels.Event[domain.AgentChat]{}, fmt.Errorf("agentchat: send: %w", apperr.ErrUnavailable)
		case errors.Is(err, asynxModels.ErrPipelineFailed):
			lastErr = err
		default:
			return asynxModels.Event[domain.AgentChat]{}, err
		}
	}
	return asynxModels.Event[domain.AgentChat]{}, lastErr
}

// sendWithOCC dispatches cmd to the singleton axAgentChat with OCC retry.
func (r *eventSourced) sendWithOCC(
	ctx context.Context,
	cmd asynxModels.Command[domain.AgentChat],
) (asynxModels.Event[domain.AgentChat], error) {
	return occSend(ctx, r.ax.Send, cmd)
}

// Create is deliberately the ONLY command on the SendWait path: it returns once
// the read model reflects the new chat, so the hook that lands microseconds later
// (a /clear mints a chat and the CLI's next prompt arrives immediately) cannot see
// "no such chat" and drop the turn. See the EventStore interface doc.
//
// The turn commands stay on the async Send path on purpose: StartTurn/StopTurn fire
// on EVERY hook, and one of their projections (the workspace working overlay) can
// run a `git merge-tree` subprocess under the per-clone git mutex on a transition —
// blocking a hook on that is the exact contention shape behind this repo's history
// of git-mutex hangs. They validate against the event log, which is synchronous, so
// they need no read-model barrier anyway.
func (r *eventSourced) Create(
	ctx context.Context,
	in CreateInput,
) (domain.AgentChat, error) {
	evt, err := occSend(ctx, r.ax.SendWait, commands.Create{
		ID:          in.ID,
		WorkspaceID: in.WorkspaceID,
		Now:         in.Now,
	})
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) StartTurn(
	ctx context.Context,
	chatID string,
	now time.Time,
) (domain.AgentChat, error) {
	evt, err := r.sendWithOCC(ctx, commands.StartTurn{ChatID: chatID, Now: now})
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: start turn: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) StopTurn(
	ctx context.Context,
	chatID string,
	now time.Time,
	asyncWork int,
) (domain.AgentChat, error) {
	evt, err := r.sendWithOCC(ctx, commands.StopTurn{ChatID: chatID, Now: now, AsyncWork: asyncWork})
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: stop turn: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) AbandonTurn(
	ctx context.Context,
	chatID string,
	now time.Time,
) (domain.AgentChat, error) {
	evt, err := r.sendWithOCC(ctx, commands.StopTurn{ChatID: chatID, Now: now, Abandoned: true})
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: abandon turn: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) SetTitle(
	ctx context.Context,
	chatID string,
	title string,
	source string,
) (domain.AgentChat, error) {
	evt, err := r.sendWithOCC(ctx, commands.SetTitle{ChatID: chatID, Title: title, Source: source})
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: set title: %w", err)
	}
	return evt.Aggregate, nil
}

// Forget purges the chat aggregate via ax.Forget (hard delete), mirroring
// reviewthread's DeleteThread. See the EventStore interface doc for details.
func (r *eventSourced) Forget(
	ctx context.Context,
	id string,
) error {
	if err := r.ax.Forget(ctx, id); err != nil {
		return fmt.Errorf("agentchat: forget: %w", err)
	}
	return nil
}

func (r *eventSourced) GetChat(
	ctx context.Context,
	id string,
) (domain.AgentChat, error) {
	chat, err := r.store.GetChat(ctx, id)
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: get chat: %w", mapNotFound(err))
	}
	return chat, nil
}

func (r *eventSourced) ListChats(
	ctx context.Context,
) ([]domain.AgentChat, error) {
	rows, err := r.store.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentchat: list chats: %w", err)
	}
	return rows, nil
}

func (r *eventSourced) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.AgentChat, error) {
	rows, err := r.store.ListByWorkspace(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("agentchat: list by workspace: %w", err)
	}
	return rows, nil
}

// mapNotFound bridges the store package's local ErrNotFound sentinel (kept
// local there to avoid an import cycle back into this package) to this
// package's own ErrNotFound, so every EventStore caller sees one sentinel
// regardless of which read path served the miss.
func mapNotFound(
	err error,
) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
