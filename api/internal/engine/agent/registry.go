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
	segToSession  map[string]string   // segment id -> last session id seen
	segToChat     map[string]string   // segment id -> chat it currently hosts
	sessionToChat map[string]string   // known session id -> chat id
	segToInjected map[string][]string // segment id -> everything Crowbar injected at spawn
}

func NewRegistry() *Registry {
	return &Registry{
		segToSession:  map[string]string{},
		segToChat:     map[string]string{},
		sessionToChat: map[string]string{},
		segToInjected: map[string][]string{},
	}
}

// SetInjectedContext records everything Crowbar injected into a segment at spawn —
// the {context} document AND the pointer message a provider reachable only through
// a user message receives — so ConsumeInjectedContext can recognise whichever of
// them comes back through the user-prompt hook.
func (r *Registry) SetInjectedContext(segmentID string, docs ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range docs {
		if d != "" {
			r.segToInjected[segmentID] = append(r.segToInjected[segmentID], d)
		}
	}
}

// ConsumeInjectedContext reports whether text is the very context document
// Crowbar injected into this segment at spawn — i.e. the CLI is echoing our own
// handoff back at us through its user-prompt hook, not relaying something the
// user typed. Provider-agnostic on purpose: a provider whose only resume channel
// is a user message (codex) fires user_prompt for the injected document, while
// one with a silent channel (claude) never does, and the registry does not need
// to know which is which.
//
// Recording that echo as a ledger turn is what made handoffs NEST — the blob
// became a "user" turn, and the next handoff embedded it inside itself. The
// match is one-shot (the entry is consumed) so that a user who genuinely retypes
// the same text later is still recorded.
func (r *Registry) ConsumeInjectedContext(segmentID, text string) bool {
	if text == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	docs := r.segToInjected[segmentID]
	for i, d := range docs {
		if d != text {
			continue
		}
		r.segToInjected[segmentID] = append(docs[:i:i], docs[i+1:]...)
		return true
	}
	return false
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

// ChatFor returns the chat a segment currently belongs to, if the registry
// knows it. A segment is bound to its chat at spawn (BindSegment) and re-bound
// by OnSessionStart when the live process moves (focus/registered), so this
// always reflects the chat currently hosting the segment. The usecase routes an
// incoming hook — which carries only its crowbarSegID — to its chat through
// this, replacing the retired GetActiveSegmentByCrowbarID DB lookup. Returns
// (,false) for a crowbarSegID the registry has never seen (an unknown/dead
// segment), so the caller ignores such a hook.
func (r *Registry) ChatFor(segmentID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	chatID, ok := r.segToChat[segmentID]
	return chatID, ok
}

// ForgetChat removes every mapping that points at chatID — the segment->chat and
// segment->session entries for all of the chat's segments, plus its session->chat
// entries. Called synchronously when a chat is hard-deleted (PurgeChat / the
// workspace-delete cascade) so the async PTY-teardown reconcile
// (reconcileSegmentExit) sees ChatFor(segID) return !ok and no-ops at its FIRST,
// in-memory guard — before the racy read-model GetChat. Bus event delivery is
// async (asynx spawns a goroutine per handler), so onForget's read-model row
// delete races reconcile's EndSegment Save; this synchronous unbind is the only
// order-independent way to stop a fast-exiting CLI from resurrecting the deleted
// chat's read row with an "ended" segment.
func (r *Registry) ForgetChat(chatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for seg, c := range r.segToChat {
		if c == chatID {
			delete(r.segToChat, seg)
			delete(r.segToSession, seg)
			delete(r.segToInjected, seg)
		}
	}
	for sess, c := range r.sessionToChat {
		if c == chatID {
			delete(r.sessionToChat, sess)
		}
	}
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
