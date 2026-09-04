package main

import (
	"context"
	"fmt"
	"time"
)

// Repo and workspace creation answer 202 and finish in the background, so a
// created entity only shows up on a later GET. The wait is bounded so a create
// that failed after the 202 names what never arrived instead of hanging.
const (
	appearTimeout  = 30 * time.Second
	appearInterval = 250 * time.Millisecond
)

type daemon struct {
	wire transport
}

// postAccepted issues a fire-and-forget mutation. The 202 routes answer with an
// empty body, so there is nothing to decode — only a status to check.
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

// patchData issues a synchronous PATCH mutation and decodes its response —
// the branch rename this tool runs is answered in place, not fire-and-forget.
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

// waitFor polls a list endpoint until pick recognises the entity, then returns
// it. what names the entity in the timeout error, so a failed async create is
// reported as the thing that is missing rather than as a bare deadline.
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
