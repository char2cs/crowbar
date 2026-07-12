package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventKeyPrefix is the namespace asynx prepends to an aggregate id when it
// stores that aggregate's events (its snapshots use "snapshots:"). The crowbar
// event store's AggregateLister returns the RAW store keys, so the heal must
// keep only the events keys and strip this prefix to recover the real aggregate
// id, which asynx.Replay re-prepends itself.
const eventKeyPrefix = "events:"

// healConversations rebuilds the APPEND-ONLY conversation history from the event
// log when that history has been lost, and does nothing else. It runs ONCE, at
// construction, and only when the conversation table is empty while the log is
// not: lose this table and chats become unresumable, and a later /resume of a
// forgotten conversation would mint a duplicate chat.
//
// It deliberately CANNOT touch agent_runners. The fold is historyProjector, which
// has no live-row writer at all — so the failure mode of a general replay (write
// a started runner's live row, then delete it on its exit event, leaving a live
// row for a dead CLI if the loop errors or the ctx cancels in between) is not
// merely avoided here, it is unrepresentable. A PTY never survives a restart, so
// there is no live runner in the log worth resurrecting anyway.
//
// Best-effort on capability: an event store that cannot enumerate its aggregates
// simply yields no heal. A store that CAN enumerate but fails is a hard error —
// silently booting on half a history is how duplicate chats get minted.
func healConversations(
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[domain.AgentRunner],
) error {
	ctx := context.Background()
	empty, err := historyIsEmpty(ctx, db)
	if err != nil {
		return err
	}
	if !empty {
		return nil
	}
	lister, ok := es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("agentrunner store: enumerate aggregate ids: %w", err)
	}
	return replayHistory(ctx, db, ax, keys)
}

// replayHistory folds every runner aggregate in the log through the history-only
// projector. It replays with a BARE projector rather than through the bus, so a
// heal never re-broadcasts historical WS frames at a frontend that has been
// watching the live stream all along.
func replayHistory(
	ctx context.Context,
	db *gormdb.DB,
	ax asynx.Asynx[domain.AgentRunner],
	keys []string,
) error {
	fold := (&historyProjector{db: db}).onEvent
	for _, key := range keys {
		id, found := strings.CutPrefix(key, eventKeyPrefix)
		if !found {
			continue
		}
		if err := ax.Replay(ctx, id, 1, 0, fold); err != nil {
			return fmt.Errorf("agentrunner store: replay %s: %w", id, err)
		}
	}
	return nil
}

func historyIsEmpty(
	ctx context.Context,
	db *gormdb.DB,
) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&conversationRow{}).Count(&count).Error; err != nil {
		return false, fmt.Errorf("agentrunner store: count conversations: %w", err)
	}
	return count == 0, nil
}

type historyProjector struct {
	db *gormdb.DB
}

func (p *historyProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.AgentRunner],
) {
	appendConversation(ctx, p.db, evt.Aggregate)
}
