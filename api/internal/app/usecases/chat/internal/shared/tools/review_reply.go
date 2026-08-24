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

// replyToReviewThread looks the thread up before writing to it: a thread id
// names a thread in SOME workspace, so the id by itself authorizes nothing. Only
// once the thread's actual WsID is known and confirmed visible to the caller
// does the reply reach the store — a rejection here must never call
// deps.ThreadWrites.Reply at all.
//
// The lookup and the scope check run on EVERY call, including one a key
// deduplicates. Answering a retry from the map before checking who is asking would
// turn a cached key into a way to learn that a thread exists in a workspace the
// caller cannot see; authorization is not a thing a retry gets to skip.
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
	// Only a real write is announced, exactly as in post_review_comment: a dedup hit
	// changed nothing, so a frame for it would tell every connected client to
	// re-render a thread it already has.
	if out.created {
		broadcastThreadWrite(deps, c, thread.WsID, out.fresh)
	}
	// The thread named is the one the reply actually LANDED on, read back off the
	// outcome rather than off the arguments: a retry that reuses a key against a
	// different thread wrote nothing, and echoing its own argument would report a
	// reply on a thread that never received one.
	return fmt.Sprintf("Replied to review thread %s.", out.threadID), nil
}

// replyInputFor builds the reply write, attributing it exactly the way
// openInputFor attributes a new thread: provider and chat come off the resolved
// Caller and never off the tool's arguments, so a model cannot file a reply under
// another agent's name or another conversation.
//
// The thread this reply lands on may belong to a workspace other than the
// caller's own — CanSee spans descendants — so the attribution deliberately
// describes the WRITER, not the thread's workspace.
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
