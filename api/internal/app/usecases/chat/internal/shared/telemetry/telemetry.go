// Package telemetry holds the last usage report each chat's provider sent.
//
// It optionally persists that report so it survives a daemon restart: a
// report that outlived the process it described used to be a number the UI
// showed and nobody could explain, but the DTO already carries ObservedAt, so
// staleness is expressible — losing the number entirely on every restart is
// worse for the user than showing a timestamped last-known value. What this
// does NOT do is invent one: a chat nothing has ever reported for stays
// unknown, not zero, restart or not (see Get's own doc).
package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Persistence is telemetry's seam onto durable storage. Satisfied directly by
// store.Store[domain.AgentChatTelemetry, string] — narrowed to the three
// methods this package actually calls so it depends on the shape it uses, not
// the generic store package.
type Persistence interface {
	Save(ctx context.Context, row domain.AgentChatTelemetry) error
	Delete(ctx context.Context, chatID string) error
	FindAll(ctx context.Context) ([]domain.AgentChatTelemetry, error)
}

// Store is the per-chat telemetry cache. Reads are frequent (every poll of a
// chat's header) and writes are rare (one per telemetry hook), so it takes a
// read/write lock rather than a plain mutex. Get never touches persist — only
// Set, Forget and construction do — so a durable Store costs a poll nothing.
type Store struct {
	mu      sync.RWMutex
	reports map[string]engineagents.Telemetry
	persist Persistence
}

// New returns a process-local-only store: nothing survives a restart. Used by
// every test and by any caller with no durable backing.
func New() *Store {
	return &Store{reports: map[string]engineagents.Telemetry{}}
}

// NewDurable returns a Store seeded from persist's rows, that writes every
// Set/Forget through to it — so a chat's last report survives a daemon
// restart. persist may be nil, in which case this behaves exactly like New:
// the one seam production and every non-durable test share.
//
// A hydration failure is logged and swallowed: it must not stop the daemon
// from starting, it just means every chat opens gauge-less until its provider
// reports again — no worse than the old, always-empty boot.
func NewDurable(ctx context.Context, persist Persistence) *Store {
	s := &Store{reports: map[string]engineagents.Telemetry{}, persist: persist}
	if persist == nil {
		return s
	}
	rows, err := persist.FindAll(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry: hydrate", "err", err)
		return s
	}
	for _, row := range rows {
		var report engineagents.Telemetry
		if err := json.Unmarshal([]byte(row.ReportJSON), &report); err != nil {
			slog.ErrorContext(ctx, "telemetry: decode", "chat_id", row.ChatID, "err", err)
			continue
		}
		s.reports[row.ChatID] = report
	}
	return s
}

// Set replaces the chat's report, in memory and — when durable — on disk. A
// provider restates its whole telemetry on every report, so there is nothing
// to merge. The durable write is best-effort: a failure is logged, not
// returned, because the in-memory report (which IS the read path) is already
// correct either way, and the next report retries the write regardless.
func (s *Store) Set(ctx context.Context, chatID string, report engineagents.Telemetry) {
	s.mu.Lock()
	s.reports[chatID] = report
	s.mu.Unlock()
	if s.persist == nil {
		return
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry: encode", "chat_id", chatID, "err", err)
		return
	}
	row := domain.AgentChatTelemetry{ChatID: chatID, ReportJSON: string(encoded), ObservedAt: report.ObservedAt}
	if err := s.persist.Save(ctx, row); err != nil {
		slog.ErrorContext(ctx, "telemetry: persist", "chat_id", chatID, "err", err)
	}
}

// Get returns the chat's last report. ok is false when no provider has reported
// for the chat in this process, which is not the same as a zero report.
func (s *Store) Get(chatID string) (engineagents.Telemetry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, ok := s.reports[chatID]
	return report, ok
}

// Forget drops the chat's report, in memory and — when durable — on disk. A
// purged chat must not leave a number behind, on this boot or the next one.
func (s *Store) Forget(ctx context.Context, chatID string) {
	s.mu.Lock()
	delete(s.reports, chatID)
	s.mu.Unlock()
	if s.persist == nil {
		return
	}
	if err := s.persist.Delete(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "telemetry: forget", "chat_id", chatID, "err", err)
	}
}
