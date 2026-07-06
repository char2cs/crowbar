package agent

import "sync"

type Outcome struct {
	Kind      string // noop | bound | focus | registered
	ChatID    string
	SessionID string
	SegmentID string
}

// Registry is the single serialized owner of context-move detection state. All
// mutation goes through one mutex so the registry can never corrupt.
type Registry struct {
	mu            sync.Mutex
	segToSession  map[string]string // segment id -> last session id seen
	segToChat     map[string]string // segment id -> chat it currently hosts
	sessionToChat map[string]string // known session id -> chat id
}

func NewRegistry() *Registry {
	return &Registry{
		segToSession:  map[string]string{},
		segToChat:     map[string]string{},
		sessionToChat: map[string]string{},
	}
}

// BindSegment records the chat a freshly-spawned segment belongs to, before its
// first SessionStart hook arrives.
func (r *Registry) BindSegment(segmentID, chatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.segToChat[segmentID] = chatID
}

// Seed marks a session id as known (used to rehydrate from the DB at startup).
func (r *Registry) Seed(sessionID, chatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionToChat[sessionID] = chatID
}

// OnSessionStart is the spec §7 reducer. It branches ONLY on facts: (1) did the
// session id under this segment change, (2) is the new id known.
func (r *Registry) OnSessionStart(segmentID, sessionID string, newChatID func() string) Outcome {
	r.mu.Lock()
	defer r.mu.Unlock()

	prev := r.segToSession[segmentID]
	switch {
	case sessionID == prev:
		return Outcome{Kind: "noop", SegmentID: segmentID, SessionID: sessionID}

	case prev == "":
		// First id for this segment: bind it to the segment's chat (spawn / switch-continuation).
		chatID := r.segToChat[segmentID]
		r.sessionToChat[sessionID] = chatID
		r.segToSession[segmentID] = sessionID
		return Outcome{Kind: "bound", ChatID: chatID, SessionID: sessionID, SegmentID: segmentID}

	default:
		if chatID, known := r.sessionToChat[sessionID]; known {
			// CASE 1: moved into a chat we know -> focus it.
			r.segToChat[segmentID] = chatID
			r.segToSession[segmentID] = sessionID
			return Outcome{Kind: "focus", ChatID: chatID, SessionID: sessionID, SegmentID: segmentID}
		}
		// CASE 2: an unknown id appeared -> register a new chat.
		chatID := newChatID()
		r.sessionToChat[sessionID] = chatID
		r.segToChat[segmentID] = chatID
		r.segToSession[segmentID] = sessionID
		return Outcome{Kind: "registered", ChatID: chatID, SessionID: sessionID, SegmentID: segmentID}
	}
}
