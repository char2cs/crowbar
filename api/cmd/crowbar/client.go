package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

const clientTimeout = 30 * time.Second

type envelope[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data"`
	Error   string `json:"error"`
}

func postJSON[T any](socket, path string) (T, error) {
	var zero T
	client, err := ipc.NewClientWithTimeout(socket, clientTimeout)
	if err != nil {
		return zero, fmt.Errorf("cannot reach the crowbar daemon: %w", err)
	}
	_, body, err := client.PostJSON(context.Background(), path, nil)
	if err != nil {
		return zero, fmt.Errorf("cannot reach the crowbar daemon: %w", err)
	}
	return decodeEnvelope[T](body)
}

func decodeEnvelope[T any](body []byte) (T, error) {
	var zero T
	var env envelope[T]
	if err := json.Unmarshal(body, &env); err != nil {
		return zero, fmt.Errorf("daemon returned an unreadable response: %w", err)
	}
	if !env.Success {
		return zero, fmt.Errorf("daemon refused: %s", env.Error)
	}
	return env.Data, nil
}
