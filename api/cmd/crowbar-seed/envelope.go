package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const snippetLimit = 200

type envelope[T any] struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    T      `json:"data"`
}

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
