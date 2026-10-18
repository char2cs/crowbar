// Package store owns the agentrunner read side, folded from the AgentRunner
// event stream:
//
//  1. agent_runners (runnerRow) — LIVE rows. One row per RUNNING CLI, deleted on
//     exit. "Does a row exist for this chat" IS the liveness question: there is
//     no status column, so there is nothing to go stale. It is never rebuilt
//     from the event log: a PTY does not survive a daemon restart, so every
//     runner the log remembers is already dead.
//
//  2. agent_chat_placements (placementRow) — APPEND-ONLY placement history
//     (placements.go).
//
// Both derive independently from the same event stream, alongside the hub
// projection (hub.go) that owns WS fan-out.
package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound is returned when the read model holds no row for a query: no live
// runner for a chat or id. Defined
// locally so the parent agentrunner package (which imports this one) can bridge
// it outward to its own sentinel without an import cycle — the same layering
// agentchat keeps between its store and its event_store.go (mapNotFound).
//
// For the LIVE-runner reads this sentinel is not an error condition at all: the
// absence of a row IS the answer, and the answer is "dormant".
var ErrNotFound = errors.New("agentrunner store: not found")

// Store is the AgentRunner read side: the live-runner model and the append-only
// placement history, both projected from the runner event stream.
type Store struct {
	db *gormdb.DB
}

// New builds both read models over db, then registers the read-model projection
// (this file) and the hub broadcast projection (hub.go) on ax.
//
// watch must not be nil: the hub projection calls it on every event, so a nil
// one panics inside a projection goroutine, far from whoever built the Store.
func New(
	db *gormdb.DB,
	ax asynx.Asynx[agents.Runner],
	watch WatchFunc,
) (*Store, error) {
	if watch == nil {
		return nil, fmt.Errorf("agentrunner store: nil watch")
	}
	if err := db.AutoMigrate(&runnerRow{}, &placementRow{}); err != nil {
		return nil, fmt.Errorf("agentrunner store: migrate: %w", err)
	}
	if err := registerStoreProjection(db, ax); err != nil {
		return nil, fmt.Errorf("agentrunner store: projections: %w", err)
	}
	if err := registerHubProjection(ax, watch); err != nil {
		return nil, fmt.Errorf("agentrunner store: projections: %w", err)
	}
	return &Store{db: db}, nil
}

// newestArrivalFirst orders live runners newest start first, with the id as a
// deterministic tiebreak. A provider switch and a chat delete displace the
// outgoing CLI before any successor spawns, so two candidates on one chat exist
// only in the window a Start retires (LiveRunnersForChat).
const newestArrivalFirst = "started_at DESC, id ASC"

// LiveRunnerForChat returns the runner currently pointed at chatID, or ErrNotFound when
// the chat is dormant. There is no status column to consult: the row exists exactly while
// its CLI runs, so its presence IS liveness — and a runner Crowbar has taken OFF this chat
// (Displace) points at nothing and is not a candidate, even while its process is still
// dying.
func (s *Store) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (agents.Runner, error) {
	// "" is NOWHERE, not a chat. A displaced runner's row carries an empty chat id, so
	// without this an empty key would MATCH those rows and hand a caller back a runner that
	// is on nothing — the read model volunteering a lie, which is the shape this whole
	// model exists to delete.
	if chatID == "" {
		return agents.Runner{}, fmt.Errorf("agentrunner store: live runner for chat: %w", ErrNotFound)
	}
	var row runnerRow
	err := s.db.WithContext(ctx).
		Where("current_chat_id = ?", chatID).
		Order(newestArrivalFirst).
		Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return agents.Runner{}, fmt.Errorf("agentrunner store: live runner for chat %q: %w", chatID, ErrNotFound)
	}
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner store: live runner for chat %q: %w", chatID, err)
	}
	return row.toRunner(), nil
}

// LiveRunnersForChat returns EVERY live runner placed on chatID, newest start
// first: a Start reads it right after its write commits and retires everyone but
// itself. An empty chatID is NOWHERE, and nowhere holds nobody.
func (s *Store) LiveRunnersForChat(
	ctx context.Context,
	chatID string,
) ([]agents.Runner, error) {
	if chatID == "" {
		return nil, nil
	}
	var rows []runnerRow
	err := s.db.WithContext(ctx).
		Where("current_chat_id = ?", chatID).
		Order(newestArrivalFirst).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("agentrunner store: live runners for chat %q: %w", chatID, err)
	}
	out := make([]agents.Runner, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toRunner())
	}
	return out, nil
}

// Get returns the live runner with runnerID, or ErrNotFound once it has exited —
// the tombstone in the event log is audit, never liveness, so an exited runner is
// simply absent here.
func (s *Store) Get(
	ctx context.Context,
	runnerID string,
) (agents.Runner, error) {
	var row runnerRow
	err := s.db.WithContext(ctx).Where("id = ?", runnerID).Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return agents.Runner{}, fmt.Errorf("agentrunner store: get %q: %w", runnerID, ErrNotFound)
	}
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner store: get %q: %w", runnerID, err)
	}
	return row.toRunner(), nil
}

// AllLive returns every runner the read model believes is still running: the
// input to boot reconciliation, which asks the PTY — the sole authority — whether
// each one really is, and Exits the ones that are not.
//
// It reads the table and nothing else. An empty result is a real answer ("nothing
// is running"), which on an idle machine is the normal one: the live table is not
// durable truth to be recovered, it is a cache of what the PTYs are doing right
// now, and after a restart the honest answer is always "nothing".
func (s *Store) AllLive(
	ctx context.Context,
) ([]agents.Runner, error) {
	var rows []runnerRow
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentrunner store: all live: %w", err)
	}
	out := make([]agents.Runner, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toRunner())
	}
	return out, nil
}

// ForgetChat drops the chat's placement history — the chat delete cascade, the
// ONLY thing permitted to remove append-only history. It does not touch the
// live-runner model: a runner's row belongs to its PTY, not to any chat.
func (s *Store) ForgetChat(
	ctx context.Context,
	chatID string,
) error {
	return forgetPlacements(ctx, s.db, chatID)
}

// registerStoreProjection subscribes the read-model projection to every
// agentrunner event. It does not broadcast — hub.go owns fan-out — so the two
// derive independently from the same stream. Designed to register ONCE, on the
// singleton ax.
func registerStoreProjection(
	db *gormdb.DB,
	ax asynx.Asynx[agents.Runner],
) error {
	p := &projector{db: db}
	if _, err := ax.Subscribe(asynx.Topic("runner.*"), p.onEvent); err != nil {
		return fmt.Errorf("agentrunner store projection: subscribe: %w", err)
	}
	return nil
}

type projector struct {
	db *gormdb.DB
}

func (p *projector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[agents.Runner],
) {
	r := evt.Aggregate

	// EXITED: drop the live row. That deletion IS how the chat goes dormant —
	// there is no status to flip, so nothing is left behind that could later
	// disagree with the PTY.
	if r.ExitedAt != nil {
		if err := p.db.WithContext(ctx).Delete(&runnerRow{}, "id = ?", r.ID).Error; err != nil {
			slog.ErrorContext(ctx, "agentrunner projection: delete live row", "runner", r.ID, "err", err)
		}
		return
	}

	p.upsertLive(ctx, r)
	if err := appendPlacement(ctx, p.db, r); err != nil {
		slog.ErrorContext(ctx, "agentrunner projection: append placement",
			"chat", r.CurrentChatID, "provider", r.ProviderID, "err", err)
	}
}

// upsertLive writes the single live row for every event but EXITED.
// Placement is whatever the aggregate says — Crowbar is its only writer, so it
// cannot drift.
func (p *projector) upsertLive(
	ctx context.Context,
	r agents.Runner,
) {
	row := runnerRow{
		ID:                    r.ID,
		WorkspaceID:           r.WorkspaceID,
		ProviderID:            r.ProviderID,
		TerminalSession:       r.TerminalSession,
		CurrentChatID:         r.CurrentChatID,
		LaunchModel:           r.LaunchModel,
		LaunchEffort:          r.LaunchEffort,
		ReportedModel:         r.ReportedModel,
		ReportedEffort:        r.ReportedEffort,
		LaunchPermissionLevel: r.LaunchPermissionLevel,
		StartedAt:             r.StartedAt,
	}
	err := p.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&row).Error
	if err != nil {
		slog.ErrorContext(ctx, "agentrunner projection: upsert live row", "runner", r.ID, "err", err)
	}
}
