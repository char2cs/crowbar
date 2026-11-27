package main

import (
	"context"
	"fmt"
	"time"
)

const (
	appearTimeout  = 30 * time.Second
	appearInterval = 250 * time.Millisecond
)

type daemon struct {
	wire transport
}

func (d *daemon) postAccepted(
	ctx context.Context,
	what string,
	path string,
	body any,
) error {
	status, respBody, err := d.wire.PostJSON(ctx, path, body)
	if err != nil {
		return fmt.Errorf("seed: %s: %w", what, err)
	}
	if !okStatus(status) {
		return fmt.Errorf("seed: %s: daemon returned %d: %s", what, status, snippet(respBody))
	}
	return nil
}

func getData[T any](
	ctx context.Context,
	d *daemon,
	what string,
	path string,
) (T, error) {
	status, body, err := d.wire.Get(ctx, path)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("seed: %s: %w", what, err)
	}
	return decodeEnvelope[T](what, status, body)
}

func postData[T any](
	ctx context.Context,
	d *daemon,
	what string,
	path string,
	body any,
) (T, error) {
	status, respBody, err := d.wire.PostJSON(ctx, path, body)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("seed: %s: %w", what, err)
	}
	return decodeEnvelope[T](what, status, respBody)
}

func patchData[T any](
	ctx context.Context,
	d *daemon,
	what string,
	path string,
	body any,
) (T, error) {
	status, respBody, err := d.wire.PatchJSON(ctx, path, body)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("seed: %s: %w", what, err)
	}
	return decodeEnvelope[T](what, status, respBody)
}

func waitFor[T any](
	ctx context.Context,
	d *daemon,
	what string,
	path string,
	pick func([]T) (T, bool),
) (T, error) {
	var zero T
	deadline := time.Now().Add(appearTimeout)
	for time.Now().Before(deadline) {
		list, err := getData[[]T](ctx, d, "wait for "+what, path)
		if err != nil {
			return zero, err
		}
		if found, ok := pick(list); ok {
			return found, nil
		}
		time.Sleep(appearInterval)
	}
	return zero, fmt.Errorf("seed: %s never appeared within %s — check the daemon log", what, appearTimeout)
}
