package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools/internal/render"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The relay gives a call 120s; staying well under it leaves room for the reply.
const (
	defaultWaitSeconds = 50
	minWaitSeconds     = 1
	maxWaitSeconds     = 100
)

func waitForReviewCommentsTool(deps Deps) toolDef {
	return toolDef{
		name: "wait_for_review_comments",
		description: "Wait for the user to leave review comments on this branch. Returns at once " +
			"with every thread whose latest message is the user's and which is still unresolved; " +
			"otherwise blocks until one arrives or the timeout passes. Reply to or resolve each " +
			"thread you receive, otherwise the next call returns it again immediately. When it " +
			"times out with nothing new, call it again to keep watching.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"timeoutSeconds":{"type":"integer","minimum":1,"maximum":100,"description":"How long to wait for a comment. Defaults to 50; values outside 1-100 are clamped."}
			},
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			var in struct {
				TimeoutSeconds int `json:"timeoutSeconds"`
			}
			if err := decode(args, &in); err != nil {
				return "", err
			}
			return waitForReviewComments(ctx, deps, c, waitTimeout(in.TimeoutSeconds))
		},
	}
}

func waitTimeout(seconds int) time.Duration {
	if seconds == 0 {
		seconds = defaultWaitSeconds
	}
	return time.Duration(min(max(seconds, minWaitSeconds), maxWaitSeconds)) * time.Second
}

// waitForReviewComments returns the caller's threads that need attention, waiting
// for one if none do. Needs-attention is derived from the threads themselves —
// unresolved with a human message last — so there is no acknowledged-state to keep.
//
// It subscribes BEFORE the first look, or a comment landing between the two would
// be missed. The read model is projected independently of the notification, so on
// a wake the notified aggregates override what the listing says.
func waitForReviewComments(
	ctx context.Context,
	deps Deps,
	c Caller,
	timeout time.Duration,
) (string, error) {
	changes, cancel := deps.ThreadChanges.Watch(c.Workspace.ID)
	defer cancel()
	waitCtx, stop := context.WithTimeout(ctx, timeout)
	defer stop()

	var woken []domain.ReviewThread
	for {
		threads, err := deps.Threads.ListByWorkspace(waitCtx, c.Workspace.ID)
		if err != nil && ctx.Err() == nil && waitCtx.Err() == nil {
			return "", fmt.Errorf("agenttools: wait_for_review_comments: %w", err)
		}
		if pending := needingAttention(overlayThreads(threads, woken, c.Workspace.ID)); len(pending) > 0 {
			return renderPending(pending), nil
		}
		woken = woken[:0]
		select {
		case <-waitCtx.Done():
			if err := ctx.Err(); err != nil {
				return "", err
			}
			return "No new review comments arrived. Call wait_for_review_comments again to keep watching.", nil
		case th := <-changes:
			woken = append(woken, th)
			woken = drainChanges(changes, woken)
		}
	}
}

func drainChanges(ch <-chan domain.ReviewThread, into []domain.ReviewThread) []domain.ReviewThread {
	for {
		select {
		case th := <-ch:
			into = append(into, th)
		default:
			return into
		}
	}
}

// overlayThreads replaces listed threads with their notified aggregates (the later
// notification wins) and appends any the listing has not caught up to.
func overlayThreads(listed, woken []domain.ReviewThread, wsID string) []domain.ReviewThread {
	out := append([]domain.ReviewThread(nil), listed...)
	for _, w := range woken {
		if w.WsID != wsID {
			continue
		}
		replaced := false
		for i := range out {
			if out[i].ID == w.ID {
				out[i], replaced = w, true
				break
			}
		}
		if !replaced {
			out = append(out, w)
		}
	}
	return out
}

func needingAttention(threads []domain.ReviewThread) []domain.ReviewThread {
	var out []domain.ReviewThread
	for _, t := range threads {
		if t.IsResolved() || len(t.Messages) == 0 || t.Messages[len(t.Messages)-1].IsAgent {
			continue
		}
		out = append(out, t)
	}
	return out
}

func renderPending(threads []domain.ReviewThread) string {
	noun := "threads need"
	if len(threads) == 1 {
		noun = "thread needs"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d review %s your attention. Reply to or resolve each one.\n", len(threads), noun)
	for _, t := range threads {
		b.WriteString(render.RenderThread(t))
	}
	return b.String()
}
