// Package runner (file apiprompt.go) delivers a React-authored prompt over an
// ALREADY-established api-transport connection, and answers whether one ever
// has been — the seam submitPromptOverAPI (prompts.go) reads to decide
// between the fast dispatch-only path and a full replacement spawn, and the
// one apiOwnsThisEvent (turn/ingest.go) reads to tell a connection that is
// merely live from one that is actually carrying the current turn.
//
// Split out of apiconn.go, which establishes and pumps a connection — a
// different responsibility from dispatching down one already open.
package runner

import "context"

// pushPromptOverAPI delivers text to runnerID's live api connection, if it has
// one — no PTY restart, no new runnerID: the same connection applyAPITransport
// opened at spawn carries every message the conversation ever sends. ok=false
// means this runner has no live api connection at all (a hooks-only provider,
// or a mixed-transport one whose serve process never came up); the caller
// falls back to restart_tui exactly as it did before mixed transport existed.
func (rs *Runners) pushPromptOverAPI(
	ctx context.Context, runnerID, sessionID, cwd, text string,
) (usedSessionID string, ok bool, err error) {
	conn, ok := rs.apiConns.get(runnerID)
	if !ok {
		return "", false, nil
	}
	// The session was already established (Fresh or Resume) by
	// applyAPITransport at spawn time, before attach's argv was ever
	// rendered — this call only ever runs Action (turn/start).
	result, err := conn.driver.Dispatch(ctx, "prompt", map[string]string{
		"session_id": sessionID,
		"cwd":        cwd,
		"text":       text,
	})
	if err != nil {
		return "", true, err
	}
	// Only on SUCCESS: a failed Dispatch never reached codex, so the companion
	// PTY's own hooks (if this same text is retried down it, or if it was the
	// one actually carrying it) are still the only real record of anything.
	conn.dispatchedOverAPI.Store(true)
	return result["session_id"], true, nil
}

// HasDispatchedOverAPI reports whether runnerID's live api connection has ever
// actually carried a prompt — as opposed to merely being established (see
// apiconn's own dispatchedOverAPI field doc). False for a runner with no live
// connection at all, same as HasLiveAPIConnection.
func (rs *Runners) HasDispatchedOverAPI(runnerID string) bool {
	conn, ok := rs.apiConns.get(runnerID)
	if !ok {
		return false
	}
	return conn.dispatchedOverAPI.Load()
}
