// Package registry holds the in-memory map of live PTY sessions.
package registry

import (
	"errors"
	"sync"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

// entry pairs a live session with the chat that owns it so the registry can
// both look a session up by id and list every session for a chat.
type entry struct {
	session *session.Session
	chatID  string
}

// Registry is a mutex-guarded map of session ID → live session, with a
// secondary chat index so listings can be scoped per chat (the chat-scoped
// lifecycle topic, spec 2026-09-02-chat-scoped-api-design §4.2).
//
// The index is keyed by CHAT, not by worktree, because a terminal belongs to
// the chat that opened it: two sibling chats sharing one worktree must not see
// each other's shells.
type Registry struct {
	mu       sync.RWMutex
	sessions map[string]entry
	byChat   map[string]map[string]struct{}
}

// New constructs an empty Registry.
func New() *Registry {
	return &Registry{
		sessions: make(map[string]entry),
		byChat:   make(map[string]map[string]struct{}),
	}
}

// Add stores a session under id, recording the owning chat so the session is
// discoverable via ListByChat.
func (r *Registry) Add(
	id string,
	chatID string,
	s *session.Session,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[id] = entry{session: s, chatID: chatID}
	ids := r.byChat[chatID]
	if ids == nil {
		ids = make(map[string]struct{})
		r.byChat[chatID] = ids
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

// Remove deletes the session for id, dropping it from the chat index too.
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
	ids := r.byChat[e.chatID]
	delete(ids, id)
	if len(ids) == 0 {
		delete(r.byChat, e.chatID)
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

// ListByChat returns the active session IDs owned by chatID. It returns an
// empty slice when the chat has no live sessions.
func (r *Registry) ListByChat(
	chatID string,
) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.byChat[chatID]
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out
}

// ChatID returns the chat ID that owns the given session ID.
// Returns ("", false) if the session is not found.
func (r *Registry) ChatID(id string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.sessions[id]
	if !ok {
		return "", false
	}
	return e.chatID, true
}

// ErrSessionNotFound is returned when a session ID does not exist.
var ErrSessionNotFound = errors.New("registry: session not found")
