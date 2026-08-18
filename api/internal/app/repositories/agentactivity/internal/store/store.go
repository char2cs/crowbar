// Package store owns the AgentActivity read model: the durable, queryable
// projection of the conversation record.
//
// It registers ONE projection on the aggregate's asynx instance and exposes the
// queries every reader uses — the chat log, the handoff assembler, and the
// cross-agent MCP surface. Repair is lazy: a read that finds an empty model
// replays the event log, exactly as the chat read model does, so a deleted
// state directory costs a rebuild rather than a lost conversation.
package store

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store/internal/content"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store/internal/projections"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store/internal/storage"
	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventKeyPrefix is the namespace asynx prepends to an aggregate id when storing
// its events. The aggregate lister returns raw store keys, so a rebuild must keep
// only the events keys and strip this to recover the real id.
const eventKeyPrefix = "events:"

// Store is the read model.
type Store struct {
	storage   *storage.Store
	content   *content.Store
	projector *projections.Projector
	ax        asynx.Asynx[domain.AgentActivity]
	es        asynxModels.Store

	healOnce sync.Once
}

// New builds the read model and registers its projection.
func New(
	db *gormdb.DB,
	contentRoot string,
	ax asynx.Asynx[domain.AgentActivity],
	es asynxModels.Store,
) (*Store, error) {
	st, err := storage.New(db)
	if err != nil {
		return nil, err
	}
	blobs, err := content.New(contentRoot)
	if err != nil {
		return nil, err
	}
	s := &Store{
		storage:   st,
		content:   blobs,
		projector: projections.New(st),
		ax:        ax,
		es:        es,
	}
	if _, err := ax.Subscribe(asynx.Topic("agentactivity.*"), s.onEvent); err != nil {
		return nil, fmt.Errorf("agentactivity store: subscribe: %w", err)
	}
	if _, err := ax.OnForget(s.onForget); err != nil {
		return nil, fmt.Errorf("agentactivity store: on forget: %w", err)
	}
	return s, nil
}

// Content exposes the payload store so a reader can resolve a ref.
func (s *Store) Content() *content.Store { return s.content }

func (s *Store) onEvent(ctx context.Context, evt asynxModels.Event[domain.AgentActivity]) {
	if err := s.projector.Apply(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "agentactivity projection: apply",
			"chat", evt.Aggregate.ChatID, "event", evt.EventName, "err", err)
	}
}

func (s *Store) onForget(ctx context.Context, evt asynxModels.Event[domain.AgentActivity]) {
	chatID := evt.AggregateID
	if chatID == "" {
		chatID = evt.Aggregate.ChatID
	}
	if err := s.projector.Forget(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agentactivity projection: forget", "chat", chatID, "err", err)
	}
}

// heal replays the event log into an empty read model, at most once per process.
//
// The guard is deliberately "empty", not "missing this chat": a chat with no
// turns is an ordinary new chat, and replaying the entire log on every read of one
// would be a permanent tax. An empty model, by contrast, can only mean the state
// directory was lost.
func (s *Store) heal(ctx context.Context) {
	s.healOnce.Do(func() {
		empty, err := s.storage.Empty(ctx)
		if err != nil || !empty {
			return
		}
		if err := s.rebuild(ctx); err != nil {
			slog.ErrorContext(ctx, "agentactivity store: rebuild", "err", err)
		}
	})
}

// rebuild replays every aggregate the event log holds.
//
// asynx.Replay delivers each event in version order with the state at that
// version, so feeding it the same projector the live path uses reconstructs the
// read model exactly — including the rows for items the aggregate has long since
// dropped from its open state.
func (s *Store) rebuild(ctx context.Context) error {
	lister, ok := s.es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("agentactivity store: enumerate aggregate ids: %w", err)
	}
	for _, key := range keys {
		id, ok := strings.CutPrefix(key, eventKeyPrefix)
		if !ok {
			continue
		}
		if err := s.ax.Replay(ctx, id, 1, 0, s.foldReplayed); err != nil {
			return fmt.Errorf("agentactivity store: replay %s: %w", id, err)
		}
	}
	return nil
}

func (s *Store) foldReplayed(ctx context.Context, evt asynxModels.Event[domain.AgentActivity]) {
	if err := s.projector.Apply(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "agentactivity store: replay fold",
			"chat", evt.Aggregate.ChatID, "err", err)
	}
}

// Turns returns a chat's turns in order.
func (s *Store) Turns(
	ctx context.Context,
	chatID string,
	after, before int64,
	limit int,
) ([]domain.ActivityTurn, error) {
	s.heal(ctx)
	return s.storage.Turns(ctx, chatID, after, before, limit)
}

func (s *Store) TurnsBefore(
	ctx context.Context,
	chatID string,
	before int64,
	limit int,
) ([]domain.ActivityTurn, error) {
	s.heal(ctx)
	return s.storage.TurnsBefore(ctx, chatID, before, limit)
}

func (s *Store) TurnsSince(
	ctx context.Context,
	chatID string,
	cut time.Time,
) ([]domain.ActivityTurn, error) {
	s.heal(ctx)
	return s.storage.TurnsSince(ctx, chatID, cut)
}

func (s *Store) CountTurns(ctx context.Context, chatID string) (int64, error) {
	s.heal(ctx)
	return s.storage.CountTurns(ctx, chatID)
}

func (s *Store) LastTurnAt(ctx context.Context, chatID, providerID string) (time.Time, bool, error) {
	s.heal(ctx)
	return s.storage.LastTurnAt(ctx, chatID, providerID)
}

func (s *Store) LastTurnForSession(
	ctx context.Context,
	chatID, providerID, sessionID string,
) (time.Time, bool, error) {
	s.heal(ctx)
	return s.storage.LastTurnForSession(ctx, chatID, providerID, sessionID)
}

func (s *Store) HasTurnAtOrAfter(
	ctx context.Context,
	chatID, providerID string,
	since time.Time,
) (bool, error) {
	s.heal(ctx)
	return s.storage.HasTurnAtOrAfter(ctx, chatID, providerID, since)
}

func (s *Store) ToolCalls(
	ctx context.Context,
	chatID string,
	after int64,
	limit int,
) ([]domain.ActivityToolCall, error) {
	s.heal(ctx)
	return s.storage.ToolCalls(ctx, chatID, after, limit)
}

func (s *Store) Subagents(ctx context.Context, chatID string) ([]domain.ActivitySubagent, error) {
	s.heal(ctx)
	return s.storage.Subagents(ctx, chatID)
}

func (s *Store) Interruptions(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityInterruption, error) {
	s.heal(ctx)
	return s.storage.Interruptions(ctx, chatID)
}

// Choices returns a chat's prompts, pending and resolved alike.
func (s *Store) Choices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error) {
	s.heal(ctx)
	return s.storage.Choices(ctx, chatID)
}

// PendingChoices returns only the prompts a chat is still waiting on.
func (s *Store) PendingChoices(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityChoice, error) {
	s.heal(ctx)
	return s.storage.PendingChoices(ctx, chatID)
}

func (s *Store) RecentToolCalls(
	ctx context.Context,
	chatIDs []string,
	since time.Time,
	limit int,
) ([]domain.ActivityToolCall, error) {
	s.heal(ctx)
	return s.storage.RecentToolCalls(ctx, chatIDs, since, limit)
}

// DeleteChat removes a chat's rows directly. It is the purge path's companion to
// asynx Forget, for the case where the aggregate is dropped without an event.
func (s *Store) DeleteChat(ctx context.Context, chatID string) error {
	return s.storage.DeleteChat(ctx, chatID)
}
