// Package runner (file apiprompt.go) delivers a React-authored prompt over an
// already-established api-transport connection.
package runner

import "context"

// pushPromptOverAPI delivers text to runnerID's live api connection, if it has
// one — no restart, no new runner. ok=false means the runner has no live
// connection (a hooks-channel runner); the caller restarts the TUI instead.
func (rs *Runners) pushPromptOverAPI(
	ctx context.Context, runnerID, sessionID, cwd, text string,
) (usedSessionID string, ok bool, err error) {
	conn, ok := rs.apiConns.get(runnerID)
	if !ok {
		return "", false, nil
	}
	// The session was established (Fresh or Resume) at spawn; this only ever
	// runs the prompt's action step.
	result, err := conn.driver.Dispatch(ctx, "prompt", map[string]string{
		"session_id": sessionID,
		"cwd":        cwd,
		"text":       text,
	})
	if err != nil {
		return "", true, err
	}
	return result["session_id"], true, nil
}
