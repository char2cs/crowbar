package runner

import "context"

func (rs *Runners) pushPromptOverAPI(
	ctx context.Context, runnerID, sessionID, cwd, text string,
) (usedSessionID string, ok bool, err error) {
	conn, ok := rs.apiConns.get(runnerID)
	if !ok {
		return "", false, nil
	}
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
