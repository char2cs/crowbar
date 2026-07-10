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

// CreateInput seeds a new AgentChat with its first (active) segment.
type CreateInput struct {
	ID               string
	WorkspaceID      string
	SegmentID        string
	CrowbarSegmentID string
	ProviderID       string
	TerminalSession  string
	Now              time.Time
}

// OpenSegmentInput opens a new active segment on an existing chat (switch-in
// or resume). Rejected with ErrValidation when an active segment already
// exists — EndSegment must close the current one first.
type OpenSegmentInput struct {
	ChatID           string
	SegmentID        string
	CrowbarSegmentID string
	ProviderID       string
	TerminalSession  string
	Now              time.Time
}

// EventStore is the asynx-backed AgentChat aggregate repository: mutations
// dispatch the command layer with optimistic-concurrency retry (sendWithOCC),
// reads delegate to the store package's read-model projection. It is the sole
// AgentChat repository — the agent usecase sends every mutation through it (the
// gorm-backed Store was retired in the Task 10 cutover).
type EventStore interface {
	Create(
		ctx context.Context,
		in CreateInput,
	) (domain.AgentChat, error)
	OpenSegment(
		ctx context.Context,
		in OpenSegmentInput,
	) (domain.AgentChat, error)
	EndSegment(
		ctx context.Context,
		chatID string,
		segmentID string,
		now time.Time,
	) (domain.AgentChat, error)
	BindSession(
		ctx context.Context,
		chatID string,
		crowbarSegmentID string,
		providerSessionID string,
	) (domain.AgentChat, error)
	StartTurn(
		ctx context.Context,
		chatID string,
		now time.Time,
	) (domain.AgentChat, error)
	StopTurn(
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
	Delete(
		ctx context.Context,
		id string,
	) error
	// Forget purges the chat aggregate outright via ax.Forget: its synchronous
	// OnForget drops the read-model row AND the underlying event log is
	// erased, so a subsequent GetChat/ListByWorkspace genuinely reports not
	// found — unlike Delete's soft tombstone, which keeps the aggregate
	// readable by direct GetChat. Used ONLY by the workspace-delete cascade
	// (repositories.Container.forgetAgentChats, Task 12): once the owning
	// workspace itself is gone, the chat has nowhere left to live. Mirrors
	// reviewthread's DeleteThread.
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
	GetByProviderSession(
		ctx context.Context,
		providerSessionID string,
	) (domain.AgentChat, error)
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

func (r *eventSourced) Create(
	ctx context.Context,
	in CreateInput,
) (domain.AgentChat, error) {
	evt, err := r.sendWithOCC(ctx, commands.Create{
		ID:               in.ID,
		WorkspaceID:      in.WorkspaceID,
		SegmentID:        in.SegmentID,
		CrowbarSegmentID: in.CrowbarSegmentID,
		ProviderID:       in.ProviderID,
		TerminalSession:  in.TerminalSession,
		Now:              in.Now,
	})
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) OpenSegment(
	ctx context.Context,
	in OpenSegmentInput,
) (domain.AgentChat, error) {
	evt, err := r.sendWithOCC(ctx, commands.OpenSegment{
		ChatID:           in.ChatID,
		SegmentID:        in.SegmentID,
		CrowbarSegmentID: in.CrowbarSegmentID,
		ProviderID:       in.ProviderID,
		TerminalSession:  in.TerminalSession,
		Now:              in.Now,
	})
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: open segment: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) EndSegment(
	ctx context.Context,
	chatID string,
	segmentID string,
	now time.Time,
) (domain.AgentChat, error) {
	evt, err := r.sendWithOCC(ctx, commands.EndSegment{ChatID: chatID, SegmentID: segmentID, Now: now})
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: end segment: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) BindSession(
	ctx context.Context,
	chatID string,
	crowbarSegmentID string,
	providerSessionID string,
) (domain.AgentChat, error) {
	evt, err := r.sendWithOCC(ctx, commands.BindSession{
		ChatID:            chatID,
		CrowbarSegmentID:  crowbarSegmentID,
		ProviderSessionID: providerSessionID,
	})
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: bind session: %w", err)
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
) (domain.AgentChat, error) {
	evt, err := r.sendWithOCC(ctx, commands.StopTurn{ChatID: chatID, Now: now})
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: stop turn: %w", err)
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

// Delete tombstones the chat (Status=deleted) via the command layer, unlike
// reviewthread's DeleteThread which hard-deletes via ax.Forget — an agentchat
// tombstone must remain readable by direct GetChat (spec: deletion removes a
// chat from list views, not from by-id lookup).
func (r *eventSourced) Delete(
	ctx context.Context,
	id string,
) error {
	if _, err := r.sendWithOCC(ctx, commands.Delete{ChatID: id}); err != nil {
		return fmt.Errorf("agentchat: delete: %w", err)
	}
	return nil
}

// Forget purges the chat aggregate via ax.Forget (hard delete), mirroring
// reviewthread's DeleteThread. See the EventStore interface doc for why this
// is distinct from Delete's soft tombstone.
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

func (r *eventSourced) GetByProviderSession(
	ctx context.Context,
	providerSessionID string,
) (domain.AgentChat, error) {
	chat, err := r.store.GetByProviderSession(ctx, providerSessionID)
	if err != nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat: get by provider session: %w", mapNotFound(err))
	}
	return chat, nil
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
