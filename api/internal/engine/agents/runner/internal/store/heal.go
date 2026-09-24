package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// eventKeyPrefix is the namespace asynx prepends to an aggregate id when it
// stores that aggregate's events (its snapshots use "snapshots:"). The crowbar
// event store's AggregateLister returns the RAW store keys, so the heal must
// keep only the events keys and strip this prefix to recover the real aggregate
// id, which asynx.Replay re-prepends itself.
const eventKeyPrefix = "events:"

// healHistory rebuilds the APPEND-ONLY history projections from the event log
// when this read DB has never built them before, and does nothing else. It runs
// ONCE, at construction, per projection. Lose the conversation history and chats
// become unresumable, and a later /resume of a forgotten conversation mints a
// duplicate chat; lose the placement history and a chat whose provider announced
// no conversation has nothing left that can name it.
//
// It heals on a missing MARKER, never on an empty table, and each projection has
// its OWN marker. The two are different facts. The only thing that ever empties
// these tables is ForgetChat, the chat-delete cascade — and runner aggregates are
// never Forgotten, so the event log keeps every placement and every
// (chat, session) pair forever. Healing an empty table would therefore resurrect
// the history of every hard-deleted chat the moment a user deleted their last
// one, and ChatForSession would start resolving sessions to chat ids that no
// longer exist: the dangling chat on /resume that this heal exists to PREVENT.
// (Exiting the runner in the cascade does not help — an exited runner's events
// stay in the log and still carry the deleted chat's history. Only the marker can
// tell "never populated" from "emptied on purpose".)
//
// A marker PER projection is what lets a new one be added at all. A read DB built
// before the placement projection existed carries the conversation marker and no
// placement table, and a single shared marker would report that DB "built" and
// leave every chat already on disk unresolvable forever — the very population the
// projection was added to recover. With its own marker the placement fold runs
// once on that DB, folding history that is already there; it rewrites no stored
// data and no aggregate is touched.
//
// Each marker is written AFTER a FULLY successful fold — including every row it
// wrote (historyProjector records a failed append; replayHistory refuses to
// return nil if any did). A heal that lost rows is a failed heal: without that,
// one transient sqlite write error would lose that history, return nil, and mark
// the read model built forever. Retrying a partial heal on the next boot is safe
// because both appends are idempotent on their composite keys.
//
// Accepted limit: "deleted stays deleted" holds across an ORDINARY reboot, not
// across a genuine read-DB loss, and not for a projection added after the fact. A
// marker dies with the DB it lives in, and the runner event log has no record of
// deletions — so a heal after a total read-DB loss, or the first placement heal on
// an existing DB, will bring back history for chats the user had deleted. Those
// rows describe chat ids nothing can look up any more (every caller resolves the
// chat itself first), and the alternative — never healing — loses resumability for
// every chat, which is worse.
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
func healHistory(
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[agents.Runner],
) error {
	ctx := context.Background()
	pending, err := pendingFolds(ctx, db)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	skipped, err := replayHistory(ctx, db, es, ax, pending)
	if err != nil {
		return err
	}
	if skipped {
		// At least one aggregate could not Replay (logged in replayHistory) — the
		// markers stay UNSET so the next boot retries them (both appends are
		// idempotent, so re-healing the aggregates that DID succeed is free). New
		// still returns nil: the runners that healed are real, usable history NOW,
		// and one unreadable aggregate must not keep the daemon from starting —
		// that is the same whole-history-for-one-bad-id loss this heal exists to
		// prevent, just moved to a different chokepoint.
		return nil
	}
	return markBuilt(ctx, db, pending)
}

// historyFold is one append-only projection the heal can rebuild: the marker
// recording that it has been built, and the append that builds it. Adding a
// projection here is all it takes to have an existing read DB fold it once.
type historyFold struct {
	markerID string
	append   func(ctx context.Context, db *gormdb.DB, r agents.Runner) error
}

// allHistoryFolds is every append-only projection the heal knows how to rebuild.
var allHistoryFolds = []historyFold{
	{markerID: conversationsMarkerID, append: appendConversation},
	{markerID: placementsMarkerID, append: appendPlacement},
}

func pendingFolds(
	ctx context.Context,
	db *gormdb.DB,
) ([]historyFold, error) {
	var pending []historyFold
	for _, fold := range allHistoryFolds {
		built, err := readModelWasBuilt(ctx, db, fold.markerID)
		if err != nil {
			return nil, err
		}
		if !built {
			pending = append(pending, fold)
		}
	}
	return pending, nil
}

// replayHistory folds every runner aggregate in the log through the history-only
// projector, running exactly the folds the caller found PENDING. It replays with
// a BARE projector rather than through the bus, so a heal never re-broadcasts
// historical WS frames at a frontend that has been watching the live stream all
// along.
//
// asynx's fold signature cannot return an error, so the projector collects the
// first write failure and replayHistory surfaces it here — the heal is strict
// where the live projection cannot be.
//
// An aggregate whose Replay itself fails — a pre-cutover payload the current
// reducer cannot fold, or any other corrupt entry in the shared event log — is
// logged and SKIPPED, not propagated: skipped reports this so the caller can
// withhold the built markers without failing construction over it. Every OTHER
// aggregate must still heal; one bad id must not cost the whole history, which
// is the same failure this heal exists to prevent in the first place.
func replayHistory(
	ctx context.Context,
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[agents.Runner],
	folds []historyFold,
) (skipped bool, err error) {
	lister, ok := es.(aggregateLister)
	if !ok {
		return false, nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return false, fmt.Errorf("agentrunner store: enumerate aggregate ids: %w", err)
	}
	p := &historyProjector{db: db, folds: folds}
	for _, key := range keys {
		id, found := strings.CutPrefix(key, eventKeyPrefix)
		if !found {
			continue
		}
		if err := ax.Replay(ctx, id, 1, 0, p.onEvent); err != nil {
			slog.ErrorContext(ctx, "agentrunner store: replay skipped unreplayable aggregate",
				"id", id, "err", err)
			skipped = true
			continue
		}
	}
	return skipped, p.failure
}

func readModelWasBuilt(
	ctx context.Context,
	db *gormdb.DB,
	markerID string,
) (bool, error) {
	var marker healMarkerRow
	err := db.WithContext(ctx).Where("id = ?", markerID).Take(&marker).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("agentrunner store: read heal marker %q: %w", markerID, err)
	}
	return true, nil
}

func markBuilt(
	ctx context.Context,
	db *gormdb.DB,
	folds []historyFold,
) error {
	for _, fold := range folds {
		marker := healMarkerRow{ID: fold.markerID, HealedAt: time.Now().UTC()}
		err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&marker).Error
		if err != nil {
			return fmt.Errorf("agentrunner store: write heal marker %q: %w", fold.markerID, err)
		}
	}
	return nil
}

// historyProjector is the heal's fold: it writes append-only history and nothing
// else (it has no live-row writer at all — see healHistory), and it REMEMBERS a
// write failure so the heal can refuse to declare itself done.
type historyProjector struct {
	db      *gormdb.DB
	folds   []historyFold
	failure error
}

func (p *historyProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[agents.Runner],
) {
	for _, fold := range p.folds {
		err := fold.append(ctx, p.db, evt.Aggregate)
		if err != nil && p.failure == nil {
			p.failure = err
		}
	}
}

// aggregateLister is the optional capability a global event store exposes so a read
// model can enumerate every aggregate it holds, driving reconcile-on-open. An event
// store that does not implement it simply skips reconcile (best-effort).
//
// Declared here rather than imported from app/repositories/internal/serialize: the
// engine must not depend on the app layer's internals, and a one-method structural
// interface satisfies the same assertion either way.
type aggregateLister interface {
	AggregateIDs(ctx context.Context) ([]string, error)
}
