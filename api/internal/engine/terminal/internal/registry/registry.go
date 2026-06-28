// Package registry holds the in-memory map of live PTY sessions.
package registry

import (
	"errors"
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/session"
)

// entry pairs a live session with the workspace that owns it so the registry
// can both look a session up by id and list every session for a workspace.
type entry struct {
	session     *session.Session
	workspaceID string
}

// Registry is a mutex-guarded map of session ID → live session, with a
// secondary workspace index so listings can be scoped per workspace (the
// workspace-scoped lifecycle topic, spec §3).
type Registry struct {
	mu          sync.RWMutex
	sessions    map[string]entry
	byWorkspace map[string]map[string]struct{}
}

// New constructs an empty Registry.
func New() *Registry {
	return &Registry{
		sessions:    make(map[string]entry),
		byWorkspace: make(map[string]map[string]struct{}),
	}
}

// Add stores a session under id, recording the owning workspace so the session
// is discoverable via ListByWorkspace.
func (r *Registry) Add(
	id string,
	workspaceID string,
	s *session.Session,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[id] = entry{session: s, workspaceID: workspaceID}
	ids := r.byWorkspace[workspaceID]
	if ids == nil {
		ids = make(map[string]struct{})
		r.byWorkspace[workspaceID] = ids
	}
	ids[id] = struct{}{}
}

// Get retrieves the session for id.
func (r *Registry) Get(
	id string,
) (*session.Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.sessions[id]
	if !ok {
		return nil, false
	}
	return e.session, true
}

// Remove deletes the session for id, dropping it from the workspace index too.
// It is a no-op if the id is not found.
func (r *Registry) Remove(
	id string,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.sessions[id]
	if !ok {
		return
	}
	delete(r.sessions, id)
	ids := r.byWorkspace[e.workspaceID]
	delete(ids, id)
	if len(ids) == 0 {
		delete(r.byWorkspace, e.workspaceID)
	}
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

// ListByWorkspace returns the active session IDs owned by workspaceID. It
// returns an empty slice when the workspace has no live sessions.
func (r *Registry) ListByWorkspace(
	workspaceID string,
) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.byWorkspace[workspaceID]
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out
}

// WorkspaceID returns the workspace ID associated with the given session ID.
// Returns ("", false) if the session is not found.
func (r *Registry) WorkspaceID(id string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.sessions[id]
	if !ok {
		return "", false
	}
	return e.workspaceID, true
}

// ErrSessionNotFound is returned when a session ID does not exist.
var ErrSessionNotFound = errors.New("registry: session not found")
