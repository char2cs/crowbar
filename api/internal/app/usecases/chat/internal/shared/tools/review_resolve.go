package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func resolveReviewThreadTool(deps Deps) toolDef {
	return toolDef{
		name:        "resolve_review_thread",
		description: "Mark a review thread resolved. Only resolve a thread whose finding you have actually addressed.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"threadId":{"type":"string","description":"The thread to resolve. Get ids from list_review_threads."}
			},
			"required":["threadId"],
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			return resolveReviewThread(ctx, deps, c, args)
		},
	}
}

type resolveReviewThreadArgs struct {
	ThreadID string `json:"threadId"`
}

func resolveReviewThread(
	ctx context.Context,
	deps Deps,
	c Caller,
	args json.RawMessage,
) (string, error) {
	var in resolveReviewThreadArgs
	if err := decode(args, &in); err != nil {
		return "", err
	}
	thread, err := deps.Threads.Get(ctx, in.ThreadID)
	if err != nil {
		return "", fmt.Errorf("agenttools: resolve_review_thread: %w", err)
	}
	if !c.CanSee(thread.WsID) {
		return "", fmt.Errorf("agenttools: resolve_review_thread: %w", ErrOutOfScope)
	}
	updated, err := deps.ThreadWrites.Resolve(ctx, in.ThreadID)
	if err != nil {
		return "", fmt.Errorf("agenttools: resolve_review_thread: %w", err)
	}
	broadcastThreadWrite(deps, c, thread.WsID, updated)
	return fmt.Sprintf("Resolved review thread %s.", in.ThreadID), nil
}

func broadcastThreadWrite(
	deps Deps,
	c Caller,
	wsID string,
	thread domain.ReviewThread,
) {
	ws := c.Workspace
	visible, err := c.Visible()
	if err != nil {
		slog.Warn("agent tools: review thread broadcast: load the visible workspaces", "ws", wsID, "err", err)
	}
	for _, w := range visible {
		if w.ID == wsID {
			ws = w
			break
		}
	}
	deps.ThreadBroadcast(thread.NormalizedMessages(), ws.ProjectID, ws.RepoID)
}
