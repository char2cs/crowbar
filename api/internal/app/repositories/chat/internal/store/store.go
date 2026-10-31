// Package store owns the agentchat read model: the durable, queryable
// projection of the domain.Chat aggregate at state/store/agent_chat.db. Two
// projections derive from the same stream: the save-only Projection here,
// axAgentChat's durable subscriber, and the live hub projection (hub.go).
//
// Like reviewthread it does NO eager reconcile on open — a normal boot re-opens
// the durable read model with zero replay. Read-model repair is lazy: every read
// heals a lost model on first access via whole-model asynx.Replay, enumerating
// every id the event log holds. Unlike reviewthread's per-id Get (which bypasses
// the read model via the repo's direct ax.Get fold), the agentchat repo routes
// ALL reads — including per-id ones — through this store, so GetChat heals the
// model exactly like the list reads.
//
// There is no session→chat lookup here any more (the retired GetByProviderSession
// scanned the chat's segments): the chat ledger's session index answers it, long
// after the process that opened the conversation is gone.
package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventKeyPrefix is the namespace asynx prepends to an aggregate id when it
// stores that aggregate's events (its snapshots use "snapshots:"). The crowbar
// event store's AggregateLister returns the RAW store keys — both "events:<id>"
// and "snapshots:<id>" — so rebuild must keep only the events keys and strip
// this prefix to recover the real aggregate id, which asynx.Replay re-prepends
// itself.
const eventKeyPrefix = "events:"

// ErrNotFound is returned when no read-model row exists for a requested chat or
// provider session, even after a whole-model rebuild attempt. Defined locally
// (rather than importing the parent agentchat package's sentinel) because that
// package imports this store package — importing back would cycle. The parent's
// event_store.go bridges this sentinel to agentchat.ErrNotFound (mapNotFound).
var ErrNotFound = errors.New("agentchat store: not found")

// Store is the AgentChat read model: a projected, queryable view of the
// aggregate. Every read heals a lost durable model via whole-model lazy Replay
// (see rebuild) before concluding a chat or session genuinely doesn't exist.
type Store interface {
	// GetChat returns the chat for id, healing the read model first if it is
	// empty (spec §3.7).
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

// GetChat returns the chat for id via an O(1) keyed lookup — T10 puts this on
// the per-hook hot path, so it must not scan and unmarshal every row. A miss
// triggers one whole-model Replay rebuild and a re-lookup, preserving the lazy
// self-heal after a read-DB loss; only a miss that survives the rebuild is a
// genuine ErrNotFound.
func (s *service) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	// An empty id is NOWHERE, never a chat — and it must not reach the miss path below,
	// which heals the read model by REPLAYING THE ENTIRE EVENT LOG. A runner Crowbar has
	// taken off its chat carries an empty chat id, so an unguarded lookup would replay the
	// whole log on every hook of every dying CLI.
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

// healOne rebuilds a SINGLE chat's read-model row from the event log and returns
// it, or nil when the id is unknown.
//
// One id, not the whole model: asynx.Replay takes a single aggregate, and that is
// all a keyed GetChat needs. It folds NOTHING when the id is unknown or already
// Forgotten, so a miss for a deleted chat (or a stale FE request) costs one empty
// replay instead of folding every OTHER chat's entire history back through the
// read model on a hook hot path. A genuine whole-model loss still heals lazily,
// one row per GetChat, plus ListChats rebuilds the lot when the model is empty.
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

// allHealed returns every row in the durable read model, first healing it via
// whole-model lazy Replay when the model is empty but the event log still holds
// aggregates (mirrors reviewthread's List/ListOrRebuild). Every exported read
// funnels through this so a read-DB loss is repaired regardless of which method
// observes it first.
func (s *service) allHealed(
	ctx context.Context,
) ([]domain.Chat, error) {
	rows, err := s.storage.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	if err := s.rebuild(ctx); err != nil {
		return nil, err
	}
	return s.storage.FindAll(ctx)
}

// rebuild enumerates every aggregate id in the event log and Replays each into
// the read model. It is a no-op when the event store cannot enumerate its ids or
// when the log is empty (nothing to Replay), so an empty model over an empty log
// stays empty.
//
// asynx.Replay rebuilds a SINGLE aggregate id, so a lost read model cannot be
// healed by one call: rebuild enumerates every id the event log holds via the
// event store's serialize.AggregateLister capability, then Replays each id back
// into state/store/agent_chat.db.
//
// A single aggregate that fails to Replay — a pre-cutover payload the current
// reducer cannot fold, or any other corrupt entry in the shared event log — is
// logged and SKIPPED, not propagated: every other aggregate must still heal.
// Before this, one bad id emptied the whole read model, because the caller
// (ListChats/GetChat) has nothing to show while rebuild is erroring.
func (s *service) rebuild(
	ctx context.Context,
) error {
	lister, ok := s.es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("agentchat store: enumerate aggregate ids: %w", err)
	}
	for _, key := range keys {
		id, ok := strings.CutPrefix(key, eventKeyPrefix)
		if !ok {
			// A non-event key (e.g. a "snapshots:<id>" row for the same aggregate)
			// — skip it; the aggregate is rebuilt from its "events:" key.
			continue
		}
		replayErr, saveErr := s.replay(ctx, id)
		if saveErr != nil {
			return fmt.Errorf("agentchat store: heal %s: %w", id, saveErr)
		}
		if replayErr != nil {
			slog.ErrorContext(ctx, "agentchat store: replay skipped unreplayable aggregate",
				"id", id, "err", replayErr)
		}
	}
	return nil
}

// replay folds id's stream into the read model, returning Replay's error and the
// first save error apart. A Forget marker deletes the row, so a tombstoned
// stream never heals back as a ghost.
func (s *service) replay(
	ctx context.Context,
	id string,
) (replayErr, saveErr error) {
	replayErr = s.ax.Replay(ctx, id, 1, 0, func(ctx context.Context, evt asynxModels.Event[domain.Chat]) {
		switch {
		case saveErr != nil:
		case evt.EventName == asynxModels.ForgetEventName:
			saveErr = s.storage.Delete(ctx, id)
		default:
			saveErr = s.storage.Save(ctx, evt.Aggregate)
		}
	})
	return replayErr, saveErr
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
