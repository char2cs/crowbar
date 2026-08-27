package permission

import "sync"

// Store is the level a chat was seeded with at creation (from the global
// default, see the conversation package) or later switched to. It is
// in-memory only and deliberately not durable — same shape and same reasoning
// as telemetry.Store: a slot describes a live chat's current dial, not a fact
// a daemon restart should resurrect. A chat this store has never seen reports
// Guarded, the safe fallback.
type Store struct {
	mu     sync.RWMutex
	levels map[string]Level
}

func New() *Store {
	return &Store{levels: map[string]Level{}}
}

func (s *Store) Set(
	chatID string,
	level Level,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.levels[chatID] = level
}

func (s *Store) Get(
	chatID string,
) Level {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if level, ok := s.levels[chatID]; ok {
		return level
	}
	return Guarded
}

func (s *Store) Forget(
	chatID string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.levels, chatID)
}
