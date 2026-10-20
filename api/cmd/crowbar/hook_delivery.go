package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

type hookEnvelope struct {
	SegmentID  string `json:"segment_id"`
	Provider   string `json:"provider"`
	Event      string `json:"event"`
	PayloadRaw string `json:"payload_raw"`
	Project    string `json:"project"`
	Repo       string `json:"repo"`
	Workspace  string `json:"workspace"`
	CreatedAt  string `json:"created_at"`
}

// hookDeliveryAttempts and hookDeliveryRetryDelay bound a SHORT, in-process
// retry — not a durability mechanism — of a daemon that failed (5xx, or no
// answer). A 4xx is the daemon's verdict on the hook, recorded once, and final.
//
// This used to be a disk-backed spool drained by a daemon-lifetime background
// loop, globally shared across every runner on the machine. That queue had no
// per-runner isolation: one long-lived, hook-heavy session could (and did,
// live — a 30k+ envelope backlog spanning days) starve every OTHER runner's
// delivery behind it, which is strictly worse than the rare transient failure
// it was built to survive. Removed in favour of this bounded, per-invocation
// retry: a hook either gets through in a few seconds or it doesn't, but it
// can never be blocked by unrelated backlog.
const (
	hookDeliveryAttempts   = 3
	hookDeliveryRetryDelay = 400 * time.Millisecond
)

func deliverHookEnvelope(
	ctx context.Context,
	client *ipc.Client,
	envelope hookEnvelope,
) ([]byte, error) {
	status, body, err := client.PostJSON(
		ctx,
		scopedAgentPath(envelope.Project, envelope.Repo, envelope.Workspace, "/hooks"),
		envelope,
	)
	if err != nil {
		return nil, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 512 {
			msg = msg[:512]
		}
		err := fmt.Errorf("hook daemon returned HTTP %d: %s", status, msg)
		if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
			return nil, refusal{err}
		}
		return nil, err
	}
	return body, nil
}

// refusal is a hook the daemon answered 4xx: retrying it records it again.
type refusal struct{ error }

func (r refusal) Unwrap() error { return r.error }

// deliverHookEnvelopeWithRetry attempts delivery up to hookDeliveryAttempts
// times, reusing the SAME *ipc.Client (and so the same pooled connection)
// across attempts — building a fresh client per attempt is what leaked one
// goroutine and one socket per delivery in the old spool loop (see git
// history: TestRegression_DrainHookSpoolReusesOneConnectionAcrossManyEnvelopesAndTicks).
func deliverHookEnvelopeWithRetry(
	ctx context.Context,
	client *ipc.Client,
	envelope hookEnvelope,
) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < hookDeliveryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(hookDeliveryRetryDelay):
			}
		}
		body, err := deliverHookEnvelope(ctx, client, envelope)
		if err == nil || errors.As(err, new(refusal)) {
			return body, err
		}
		lastErr = err
	}
	return nil, lastErr
}
