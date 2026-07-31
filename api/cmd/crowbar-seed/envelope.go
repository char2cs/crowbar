package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const snippetLimit = 200

// envelope is the uniform v0 response body every route returns.
type envelope[T any] struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    T      `json:"data"`
}

// decodeEnvelope unwraps a v0 response into its data payload. It inspects the
// HTTP status itself because the transports return err == nil for a non-2xx: a
// caller that only checks err reads "404 not found" as a successful create.
func decodeEnvelope[T any](
	what string,
	status int,
	body []byte,
) (T, error) {
	var zero T
	if !okStatus(status) {
		return zero, fmt.Errorf("seed: %s: daemon returned %d: %s", what, status, snippet(body))
	}
	var env envelope[T]
	if err := json.Unmarshal(body, &env); err != nil {
		return zero, fmt.Errorf("seed: %s: decode response: %w: %s", what, err, snippet(body))
	}
	if !env.Success {
		return zero, fmt.Errorf("seed: %s: %s", what, env.Error)
	}
	return env.Data, nil
}

func okStatus(
	status int,
) bool {
	return status >= 200 && status < 300
}

// snippet keeps a failing body short enough to read on one screen while still
// carrying the daemon's own words about what went wrong.
func snippet(
	body []byte,
) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty body)"
	}
	if len(trimmed) > snippetLimit {
		return trimmed[:snippetLimit] + "…"
	}
	return trimmed
}
