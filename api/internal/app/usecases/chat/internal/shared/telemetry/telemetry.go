// Package telemetry holds the last usage report each chat's provider sent, optionally persisted:
// the report carries ObservedAt, so a timestamped last-known value beats losing it. A chat nothing
// ever reported for stays unknown, never zero.
package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Persistence is telemetry's seam onto durable storage, satisfied by
// store.Store[domain.AgentChatTelemetry, string] and narrowed to the methods this package calls.
type Persistence interface {
	Save(ctx context.Context, row domain.AgentChatTelemetry) error
	Delete(ctx context.Context, chatID string) error
	FindAll(ctx context.Context) ([]domain.AgentChatTelemetry, error)
}

// Store is the per-chat telemetry cache. Reads (every header poll) far outnumber writes (one per
// telemetry hook), hence the RWMutex; Get never touches persist, so durability costs a poll nothing.
type Store struct {
	mu      sync.RWMutex
	reports map[string]engineagents.Telemetry
	persist Persistence
}

// NewDurable returns a Store seeded from persist that writes every Set/Forget through to it; a nil
// persist keeps reports in memory only. A hydration failure is logged, not returned: it must not stop
// the daemon, and chats just open gauge-less until their provider reports again.
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

// Set replaces the chat's report (each report restates everything, so nothing merges). The durable
// write is best-effort: the in-memory report is the read path, and the next report retries the write.
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
