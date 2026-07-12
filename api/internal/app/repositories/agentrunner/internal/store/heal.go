package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
// log when this read DB has never been built before, and does nothing else. It
// runs ONCE, at construction. Lose that history and chats become unresumable, and
// a later /resume of a forgotten conversation mints a duplicate chat.
//
// It heals on a missing MARKER, never on an empty table. The two are different
// facts. The only thing that ever empties agent_chat_conversations is ForgetChat,
// the chat-delete cascade — and runner aggregates are never Forgotten, so the
// event log keeps every (chat, session) pair forever. Healing an empty table
// would therefore resurrect the history of every hard-deleted chat the moment a
// user deleted their last one, and ChatForSession would start resolving sessions
// to chat ids that no longer exist: the dangling chat on /resume that this heal
// exists to PREVENT. (Exiting the runner in the cascade does not help — an exited
// runner's events stay in the log and still carry the deleted chat's
// conversations. Only the marker can tell "never populated" from "emptied on
// purpose".)
//
// The marker is written AFTER a successful heal, so a heal that fails is retried
// on the next boot rather than silently skipped forever.
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
//
// It runs on context.Background(): New takes no ctx (the signature the brief
// mandates), so a first-boot heal over a very large event log is uncancellable.
func healConversations(
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[domain.AgentRunner],
) error {
	ctx := context.Background()
	built, err := readModelWasBuilt(ctx, db)
	if err != nil {
		return err
	}
	if built {
		return nil
	}
	if err := replayHistory(ctx, db, es, ax); err != nil {
		return err
	}
	return markBuilt(ctx, db)
}

// replayHistory folds every runner aggregate in the log through the history-only
// projector. It replays with a BARE projector rather than through the bus, so a
// heal never re-broadcasts historical WS frames at a frontend that has been
// watching the live stream all along.
func replayHistory(
	ctx context.Context,
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[domain.AgentRunner],
) error {
	lister, ok := es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("agentrunner store: enumerate aggregate ids: %w", err)
	}
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

func readModelWasBuilt(
	ctx context.Context,
	db *gormdb.DB,
) (bool, error) {
	var marker healMarkerRow
	err := db.WithContext(ctx).Where("id = ?", healMarkerID).Take(&marker).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("agentrunner store: read heal marker: %w", err)
	}
	return true, nil
}

func markBuilt(
	ctx context.Context,
	db *gormdb.DB,
) error {
	marker := healMarkerRow{ID: healMarkerID, HealedAt: time.Now().UTC()}
	if err := db.WithContext(ctx).Create(&marker).Error; err != nil {
		return fmt.Errorf("agentrunner store: write heal marker: %w", err)
	}
	return nil
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
