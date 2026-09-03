// Package telemetry holds the last usage report each chat's provider sent.
//
// It is process-local and deliberately not durable: a report describes a live
// CLI's context window and cost so far, and a report that outlived the process it
// described would be a number the UI shows and nobody can explain.
package telemetry

import (
	"sync"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Store is the per-chat telemetry cache. Reads are frequent (every poll of a
// chat's header) and writes are rare (one per telemetry hook), so it takes a
// read/write lock rather than a plain mutex.
type Store struct {
	mu      sync.RWMutex
	reports map[string]engineagents.Telemetry
}

// New returns an empty store.
func New() *Store {
	return &Store{reports: map[string]engineagents.Telemetry{}}
}

// Set replaces the chat's report. A provider restates its whole telemetry on
// every report, so there is nothing to merge.
func (s *Store) Set(chatID string, report engineagents.Telemetry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[chatID] = report
}

// Get returns the chat's last report. ok is false when no provider has reported
// for the chat in this process, which is not the same as a zero report.
func (s *Store) Get(chatID string) (engineagents.Telemetry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, ok := s.reports[chatID]
	return report, ok
}

// Forget drops the chat's report. A purged chat must not leave a number behind.
func (s *Store) Forget(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.reports, chatID)
}
