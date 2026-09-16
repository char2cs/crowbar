package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

type hookEnvelope struct {
	DeliveryID string `json:"delivery_id"`
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
// retry — not a durability mechanism. It exists only to absorb one specific,
// measured transient: a hook firing before the runner row it targets is
// queryable yet (see barriers_test.go's SessionStart race). Production
// self-heals that race within ~1s; three attempts a few hundred ms apart
// covers it without ever writing the event to disk.
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
		return nil, fmt.Errorf("hook daemon returned HTTP %d: %s", status, msg)
	}
	return body, nil
}

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
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
