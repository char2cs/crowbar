package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

// clientTimeout bounds a CLI command's round trip to the local daemon. A
// human waiting on a terminal can tolerate more than the in-PTY hook
// callback's own short budget (ipc.DefaultTimeout), so this is generous
// instead.
const clientTimeout = 30 * time.Second

// envelope is the {success,data,error} shape every /v0 route answers with.
type envelope[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data"`
	Error   string `json:"error"`
}

// getJSON issues a GET against the daemon's unix socket and decodes its
// envelope into T.
func getJSON[T any](socket, path string) (T, error) {
	var zero T
	client, err := ipc.NewClientWithTimeout(socket, clientTimeout)
	if err != nil {
		return zero, fmt.Errorf("cannot reach the crowbar daemon: %w", err)
	}
	_, body, err := client.Get(context.Background(), path)
	if err != nil {
		return zero, fmt.Errorf("cannot reach the crowbar daemon: %w", err)
	}
	return decodeEnvelope[T](body)
}

// postJSON issues a POST with no body against the daemon's unix socket and
// decodes its envelope into T. Every route this CLI mutates through (pairing)
// takes no request body of its own.
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
