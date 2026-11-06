// Package store owns the agentchat read model (state/store/agent_chat.db): the save-only durable
// projection beside the live hub one (hub.go). Boot does no eager reconcile; every read heals a
// lost model on first access by replaying the event log.
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/core/readonce"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventKeyPrefix is the namespace asynx prepends to an aggregate's event key.
// AggregateLister returns raw keys (events: and snapshots:), so rebuild keeps
// only events keys and strips this prefix; asynx.Replay re-prepends it.
const eventKeyPrefix = "events:"

// ErrNotFound is returned when no read-model row exists for a chat even after a
// heal. It is local because the parent package imports this one; the parent
// maps it to its own ErrNotFound.
var ErrNotFound = errors.New("agentchat store: not found")

// Store is the AgentChat read model: a projected, queryable view of the
// aggregate. Every read heals a lost durable model by lazy Replay before
// concluding a chat does not exist.
type Store interface {
	// GetChat returns the chat for id, replaying that id from the event log on a miss.
	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	// ListChats returns every chat in the durable read model, first healing it
	// via whole-model lazy Replay when the model is empty but the event log
	// still holds aggregates.
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)
	// ListByWorkspace returns the chats for wsID, healing the read model the
	// same way ListChats does before filtering.
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)
}

// service is the agentchat read model over the durable store projection. It
// holds es (the event log's serialize.AggregateLister source) and ax (for
// Replay) so reads can rebuild a lost read model on demand.
type service struct {
	storage storage
	es      asynxModels.Store
	ax      asynx.Asynx[domain.Chat]
	// rebuiltFor is the row set the last rebuild left with nothing to read. Rows
	// alone key it: a new aggregate reaches the model by its live save, not a rebuild.
	rebuiltFor atomic.Pointer[string]
}

// Projection is the read model's write side. It is opened before the asynx
// instance so it can be that instance's durable subscriber (Subscriber).
type Projection struct {
	storage storage
}

// Open opens the read model over db (state/store/agent_chat.db).
func Open(
	db *gormdb.DB,
) (*Projection, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("agentchat store: %w", err)
	}
	return &Projection{storage: st}, nil
}

// New builds the read model over p, whose instance ax was built with
// Subscriber(p), and registers the hub projection on ax. es is the log ax
// wraps, kept for the lazy whole-model rebuild.
func New(
	p *Projection,
	es asynxModels.Store,
	ax asynx.Asynx[domain.Chat],
	watch WatchFunc,
) (Store, error) {
	if err := registerHubProjection(ax, watch); err != nil {
		return nil, fmt.Errorf("agentchat store: projections: %w", err)
	}
	return &service{storage: p.storage, es: es, ax: ax}, nil
}

// GetChat returns the chat for id via a keyed lookup, since it is on the per-hook
// hot path. A miss replays that one id and looks again; only a miss that survives
// the heal is ErrNotFound.
func (s *service) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	// An empty id is never a chat and must not reach the heal path: a runner taken
	// off its chat carries an empty id, and every hook of a dying CLI would replay.
	if id == "" {
		return domain.Chat{}, fmt.Errorf("agentchat store: get %q: %w", id, ErrNotFound)
	}
	chat, err := s.storage.FindByKey(ctx, id)
	if err != nil {
		return domain.Chat{}, err
	}
	if chat == nil {
		if chat, err = s.healOne(ctx, id); err != nil {
			return domain.Chat{}, err
		}
	}
	if chat == nil {
		return domain.Chat{}, fmt.Errorf("agentchat store: get %q: %w", id, ErrNotFound)
	}
	return *chat, nil
}

// healOne rebuilds one chat's read-model row from the event log and returns it,
// or nil when the id is unknown. One id, not the whole model, so a miss for a
// deleted chat on the hook hot path costs one empty replay.
func (s *service) healOne(ctx context.Context, id string) (*domain.Chat, error) {
	if err := errors.Join(s.replay(ctx, id)); err != nil {
		return nil, fmt.Errorf("agentchat store: heal %q: %w", id, err)
	}
	return s.storage.FindByKey(ctx, id)
}

// ListChats returns every chat, healing the read model first when empty.
func (s *service) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	all, err := s.allHealed(ctx)
	if err != nil {
		return nil, err
	}
	return all, nil
}

// ListByWorkspace heals the read model (via ListChats) then filters to wsID.
func (s *service) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	all, err := s.ListChats(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Chat, 0, len(all))
	for _, chat := range all {
		if chat.WorkspaceID == wsID {
			result = append(result, chat)
		}
	}
	return result, nil
}

// allHealed returns every read-model row, first rebuilding the whole model when
// it is empty but the event log still holds aggregates.
func (s *service) allHealed(
	ctx context.Context,
) ([]domain.Chat, error) {
	rows, err := readonce.Do(ctx, "agentchat:all", func() ([]domain.Chat, error) { return s.healedRows(ctx) })
	return slices.Clone(rows), err
}

// Subscriber registers p as the instance's durable subscriber: a failed save is
// retried by asynx, and parks the aggregate, instead of being logged and
// dropped. The Forget marker deletes the row; a missing row is no error.
func Subscriber(
	p *Projection,
) func(*asynx.Builder[domain.Chat]) {
	return func(b *asynx.Builder[domain.Chat]) {
		b.WithSubscriber(asynx.Topic("agentchat.*"), p.handle)
	}
}

func (p *Projection) handle(
	ctx context.Context,
	evt asynxModels.Event[domain.Chat],
) error {
	if evt.EventName == asynxModels.ForgetEventName {
		if err := p.storage.Delete(ctx, evt.AggregateID); err != nil {
			return fmt.Errorf("agentchat store projection: delete %q: %w", evt.AggregateID, err)
		}
		return nil
	}
	if err := p.storage.Save(ctx, evt.Aggregate); err != nil {
		return fmt.Errorf("agentchat store projection: save %q: %w", evt.Aggregate.ID, err)
	}
	return nil
}
