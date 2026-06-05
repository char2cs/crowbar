// Package registry holds the in-memory map of live PTY sessions.
package registry

import (
	"errors"
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/session"
)

// Registry is a mutex-guarded map of session ID → *session.Session.
type Registry struct {
	mu       sync.RWMutex
	sessions map[string]*session.Session
}

// New constructs an empty Registry.
func New() *Registry {
	return &Registry{sessions: make(map[string]*session.Session)}
}

// Add stores a session under id.
func (r *Registry) Add(
	id string,
	s *session.Session,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[id] = s
}

// Get retrieves the session for id.
func (r *Registry) Get(
	id string,
) (*session.Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	return s, ok
}

// Remove deletes the session for id. It is a no-op if the id is not found.
func (r *Registry) Remove(
	id string,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
}

// List returns all active session IDs.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.sessions))
	for id := range r.sessions {
		ids = append(ids, id)
	}
	return ids
}

// ErrSessionNotFound is returned when a session ID does not exist.
var ErrSessionNotFound = errors.New("registry: session not found")
