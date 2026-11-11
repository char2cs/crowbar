package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools/internal/render"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
)

func replyToReviewThreadTool(deps Deps) toolDef {
	return toolDef{
		name:        "reply_to_review_thread",
		description: "Reply to an existing review thread. Get thread ids from list_review_threads.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"threadId":{"type":"string","description":"The thread to reply to. Get ids from list_review_threads."},
				"body":{"type":"string","description":"The reply, in markdown."},
				"idempotencyKey":{"type":"string","description":"Stable key for this reply; a retry with the same key will not duplicate it."}
			},
			"required":["threadId","body"],
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			return replyToReviewThread(ctx, deps, c, args)
		},
	}
}

type replyToReviewThreadArgs struct {
	ThreadID       string `json:"threadId"`
	Body           string `json:"body"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// replyToReviewThread checks the thread's own WsID against the caller before writing: an id alone
// authorizes nothing. The check runs on every call, including deduplicated retries, so a cached key
// cannot reveal a thread the caller cannot see.
func replyToReviewThread(
	ctx context.Context,
	deps Deps,
	c Caller,
	args json.RawMessage,
) (string, error) {
	var in replyToReviewThreadArgs
	if err := decode(args, &in); err != nil {
		return "", err
	}
	if err := render.CheckBody("reply_to_review_thread", in.Body); err != nil {
		return "", err
	}
	thread, err := deps.Threads.Get(ctx, in.ThreadID)
	if err != nil {
		return "", fmt.Errorf("agenttools: reply_to_review_thread: %w", err)
	}
	if !c.CanSee(thread.WsID) {
		return "", fmt.Errorf("agenttools: reply_to_review_thread: %w", ErrOutOfScope)
	}
	out, err := deps.Idempotency.replyOnce(
		ctx,
		deps.ThreadWrites,
		c.Workspace.ID,
		in.IdempotencyKey,
		replyInputFor(c, in.ThreadID, in.Body),
		time.Now(),
	)
	if err != nil {
		return "", fmt.Errorf("agenttools: reply_to_review_thread: %w", err)
	}
	// Only a real write is announced; a dedup hit changed nothing.
	if out.created {
		broadcastThreadWrite(deps, c, thread.WsID, out.fresh)
	}
	// Report the thread the reply landed on: a keyed retry naming another thread wrote nothing.
	return fmt.Sprintf("Replied to review thread %s.", out.threadID), nil
}

// replyInputFor builds the reply write, attributed like openInputFor from the resolved Caller (the
// writer, not the thread's workspace), never from the arguments.
func replyInputFor(
	c Caller,
	threadID string,
	body string,
) reviewthread.ReplyInput {
	return reviewthread.ReplyInput{
		ID:         threadID,
		MessageID:  uuid.NewString(),
		Author:     authorOf(c),
		IsAgent:    true,
		ProviderID: c.ProviderID,
		ChatID:     c.ChatID,
		Body:       body,
	}
}
