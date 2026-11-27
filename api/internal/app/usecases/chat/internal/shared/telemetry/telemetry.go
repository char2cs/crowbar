package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type Persistence interface {
	Save(ctx context.Context, row domain.AgentChatTelemetry) error
	Delete(ctx context.Context, chatID string) error
	FindAll(ctx context.Context) ([]domain.AgentChatTelemetry, error)
}

type Store struct {
	mu      sync.RWMutex
	reports map[string]engineagents.Telemetry
	persist Persistence
}

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

func (s *Store) Get(chatID string) (engineagents.Telemetry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, ok := s.reports[chatID]
	return report, ok
}

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
