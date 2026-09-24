// Package runner (file sessionorigin.go) records which conversations CROWBAR
// ITSELF originated on a runner's api connection, so the hook ingress can tell
// them from the one thing that looks identical on the wire: the user typing
// /clear into the CLI.
//
// HandleSessionStart infers a /clear from absence — an announced conversation
// Crowbar has no record of must be one the user asked for. That inference is
// only sound while Crowbar knows every conversation it opens itself, and a
// driver recovering a lost session opens one without any caller asking. It
// used to be read as a /clear, which minted a brand new chat and delivered the
// user's message into it, leaving the real chat wedged. Confirmed live.
package runner

import "sync"

// originatedSessions is one api connection's record of the conversations its
// own driver produced.
//
// pending is what makes this race-free: a claim is opened BEFORE the
// replacement exists, because a provider announces a new conversation over
// this same connection and can do so before the call that creates it has
// returned. While a claim is open, any conversation this runner announces is
// Crowbar's own doing by construction — nothing else on this connection can be
// opening one.
type originatedSessions struct {
	mu       sync.Mutex
	pending  int
	sessions map[string]struct{}
}

func newOriginatedSessions() *originatedSessions {
	return &originatedSessions{sessions: map[string]struct{}{}}
}

// Claim satisfies engineagents.APISessionOrigin.
func (o *originatedSessions) Claim() func(string) {
	o.mu.Lock()
	o.pending++
	o.mu.Unlock()
	return o.settle
}

func (o *originatedSessions) settle(sessionID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pending--
	if sessionID == "" {
		return
	}
	o.sessions[sessionID] = struct{}{}
}

func (o *originatedSessions) has(sessionID string) bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.pending > 0 {
		return true
	}
	_, ok := o.sessions[sessionID]
	return ok
}

// OriginatedSession reports whether runnerID's own api driver produced
// sessionID, or is producing one right now. False for a hooks-only runner,
// which has no connection that could.
//
// Satisfies turn.Runners: the hook ingress asks this of every api-channel
// event, because a conversation Crowbar's own driver never opened is one the
// provider opened for itself (a spawned child thread) and is not this chat's
// to record. HandleSessionStart asks it for the /clear inference.
func (rs *Runners) OriginatedSession(runnerID, sessionID string) bool {
	conn, ok := rs.apiConns.get(runnerID)
	if !ok {
		return false
	}
	return conn.originated.has(sessionID)
}
