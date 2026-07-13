// Package store owns the agentchat read model: the durable, queryable
// projection of the domain.AgentChat aggregate at state/store/agent_chat.db.
// New builds the read model over the read-pool DB and registers TWO distinct
// projections on the singleton axAgentChat (mirrors reviewthread): the
// SAVE-ONLY store projection here (store.go) folds evt.Aggregate into the
// durable read model and, on Forget, deletes the row; the hub projection
// (hub.go) owns WS fan-out, broadcasting a (chatID, kind) frame per event. The
// two derive independently from the same event stream and cannot drift.
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
// scanned the chat's segments). A conversation belongs to a chat as a projection
// of RUNNER events, so that question is answered by agentrunner's append-only
// conversation history — which keeps answering long after the process that opened
// the conversation is gone, something a scan of live chat state never could.
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
	) (domain.AgentChat, error)
	// ListChats returns every chat in the durable read model, first healing it
	// via whole-model lazy Replay when the model is empty but the event log
	// still holds aggregates.
	ListChats(
		ctx context.Context,
	) ([]domain.AgentChat, error)
	// ListByWorkspace returns the chats for wsID, healing the read model the
	// same way ListChats does before filtering.
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.AgentChat, error)
}

// service is the agentchat read model over the durable store projection. It
// holds es (the event log's serialize.AggregateLister source) and ax (for
// Replay) so reads can rebuild a lost read model on demand.
type service struct {
	storage storage
	es      asynxModels.Store
	ax      asynx.Asynx[domain.AgentChat]
}

// New builds the durable read model over db (state/store/agent_chat.db) and
// registers both read-side projections on ax, once, for the singleton
// axAgentChat: the save-only store projection (this file) and the hub
// broadcast projection (hub.go). It performs no reconcile — read-model repair
// is lazy. es is the same per-type event log ax wraps, retained so reads can
// reach its serialize.AggregateLister capability for the whole-model rebuild.
func New(
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[domain.AgentChat],
	broadcast BroadcastFunc,
) (Store, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("agentchat store: %w", err)
	}
	if err := registerStoreProjection(st, ax); err != nil {
		return nil, fmt.Errorf("agentchat store: projections: %w", err)
	}
	if err := registerHubProjection(ax, broadcast); err != nil {
		return nil, fmt.Errorf("agentchat store: projections: %w", err)
	}
	return &service{storage: st, es: es, ax: ax}, nil
}

// GetChat returns the chat for id via an O(1) keyed lookup — T10 puts this on
// the per-hook hot path, so it must not scan and unmarshal every row. A miss
// triggers one whole-model Replay rebuild and a re-lookup, preserving the lazy
// self-heal after a read-DB loss; only a miss that survives the rebuild is a
// genuine ErrNotFound.
func (s *service) GetChat(
	ctx context.Context,
	id string,
) (domain.AgentChat, error) {
	// An empty id is NOWHERE, never a chat — and it must not reach the miss path below,
	// which heals the read model by REPLAYING THE ENTIRE EVENT LOG. A runner Crowbar has
	// taken off its chat carries an empty chat id, so an unguarded lookup would replay the
	// whole log on every hook of every dying CLI.
	if id == "" {
		return domain.AgentChat{}, fmt.Errorf("agentchat store: get %q: %w", id, ErrNotFound)
	}
	chat, err := s.storage.FindByKey(ctx, id)
	if err != nil {
		return domain.AgentChat{}, err
	}
	if chat == nil {
		// Heal ONLY this id, not the whole model. asynx.Replay takes a single aggregate,
		// and that is all a keyed GetChat needs: it rebuilds THIS row if the read DB lost
		// it, and folds NOTHING when the id is unknown or already Forgotten — so a miss for
		// a deleted chat (or a stale FE request) costs one empty replay instead of folding
		// every OTHER chat's entire history back through the read model on a hook hot path.
		// A genuine whole-model loss still heals lazily, one row per GetChat, plus ListChats
		// rebuilds the lot when the model is empty.
		if err := s.ax.Replay(ctx, id, 1, 0, s.foldReplayed); err != nil {
			return domain.AgentChat{}, fmt.Errorf("agentchat store: heal %q: %w", id, err)
		}
		chat, err = s.storage.FindByKey(ctx, id)
		if err != nil {
			return domain.AgentChat{}, err
		}
	}
	if chat == nil {
		return domain.AgentChat{}, fmt.Errorf("agentchat store: get %q: %w", id, ErrNotFound)
	}
	return *chat, nil
}

// ListChats returns every chat, healing the read model first when empty.
func (s *service) ListChats(
	ctx context.Context,
) ([]domain.AgentChat, error) {
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
) ([]domain.AgentChat, error) {
	all, err := s.ListChats(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.AgentChat, 0, len(all))
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
) ([]domain.AgentChat, error) {
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
		if err := s.ax.Replay(ctx, id, 1, 0, s.foldReplayed); err != nil {
			return fmt.Errorf("agentchat store: replay %s: %w", id, err)
		}
	}
	return nil
}

// foldReplayed persists each replayed aggregate into the read model. Replay
// folds the event stream, so evt.Aggregate carries the aggregate's state at that
// version; the final event leaves the current state durable.
func (s *service) foldReplayed(
	ctx context.Context,
	evt asynxModels.Event[domain.AgentChat],
) {
	if err := s.storage.Save(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "agentchat store: replay fold", "id", evt.Aggregate.ID, "err", err)
	}
}

// registerStoreProjection subscribes the SAVE-ONLY read-model projection to
// every agentchat event on the singleton axAgentChat: it folds evt.Aggregate
// into the durable read model and, on Forget, deletes the aggregate's row (a
// cheap, synchronous OnForget row-delete, no fs/git/network io). It does NOT
// broadcast — Task 8's hub projection owns fan-out. Designed to register ONCE on
// the singleton.
func registerStoreProjection(
	st storage,
	ax asynx.Asynx[domain.AgentChat],
) error {
	p := &storeProjector{storage: st}
	if _, err := ax.Subscribe(asynx.Topic("agentchat.*"), p.onEvent); err != nil {
		return fmt.Errorf("agentchat store projection: subscribe: %w", err)
	}
	if _, err := ax.OnForget(p.onForget); err != nil {
		return fmt.Errorf("agentchat store projection: on forget: %w", err)
	}
	return nil
}

type storeProjector struct {
	storage storage
}

func (p *storeProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.AgentChat],
) {
	if err := p.saveWithRetry(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "agentchat store projection: save", "id", evt.Aggregate.ID, "err", err)
	}
}

func (p *storeProjector) saveWithRetry(
	ctx context.Context,
	chat domain.AgentChat,
) error {
	var err error
	for range 3 {
		if err = p.storage.Save(ctx, chat); err == nil {
			return nil
		}
		if !isTransientIOError(err) {
			return err
		}
	}
	return err
}

func isTransientIOError(
	err error,
) bool {
	return strings.Contains(err.Error(), "disk I/O error")
}

func (p *storeProjector) onForget(
	ctx context.Context,
	evt asynxModels.Event[domain.AgentChat],
) {
	if err := p.storage.Delete(ctx, evt.Aggregate.ID); err != nil {
		slog.ErrorContext(ctx, "agentchat store projection: delete", "id", evt.Aggregate.ID, "err", err)
	}
}
