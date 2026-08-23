package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
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
// The marker is written AFTER a FULLY successful heal — including every row the
// fold wrote (historyProjector records a failed append; replayHistory refuses to
// return nil if any did). A heal that lost rows is a failed heal: without that,
// one transient sqlite write error would lose those conversations, return nil,
// and mark the read model built forever. Retrying a partial heal on the next boot
// is safe because appendConversation is idempotent on the composite key.
//
// Accepted limit: "deleted stays deleted" holds across an ORDINARY reboot, not
// across a genuine read-DB loss. The marker dies with the DB it lives in, and the
// runner event log has no record of deletions — so a heal after a total read-DB
// loss will bring back the conversations of chats the user had deleted. That is
// the degraded case by definition (the read model is gone), and the alternative
// (never healing) loses resumability for every chat, which is worse.
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
	ax asynx.Asynx[agents.Runner],
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
//
// asynx's fold signature cannot return an error, so the projector collects the
// first write failure and replayHistory surfaces it here — the heal is strict
// where the live projection cannot be.
func replayHistory(
	ctx context.Context,
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[agents.Runner],
) error {
	lister, ok := es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("agentrunner store: enumerate aggregate ids: %w", err)
	}
	p := &historyProjector{db: db}
	for _, key := range keys {
		id, found := strings.CutPrefix(key, eventKeyPrefix)
		if !found {
			continue
		}
		if err := ax.Replay(ctx, id, 1, 0, p.onEvent); err != nil {
			return fmt.Errorf("agentrunner store: replay %s: %w", id, err)
		}
	}
	return p.failure
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
	err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&marker).Error
	if err != nil {
		return fmt.Errorf("agentrunner store: write heal marker: %w", err)
	}
	return nil
}

// historyProjector is the heal's fold: it writes conversation history and nothing
// else (it has no live-row writer at all — see healConversations), and it REMEMBERS
// a write failure so the heal can refuse to declare itself done.
type historyProjector struct {
	db      *gormdb.DB
	failure error
}

func (p *historyProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[agents.Runner],
) {
	err := appendConversation(ctx, p.db, evt.Aggregate)
	if err != nil && p.failure == nil {
		p.failure = err
	}
}
