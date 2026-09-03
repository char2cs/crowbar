package tools

import (
	"context"
	"encoding/json"
	"fmt"

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

// resolveReviewThread carries the same scope check as replyToReviewThread, and
// for the same reason: the thread id alone says nothing about who may act on it,
// only the thread's own WsID does.
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

// broadcastThreadWrite fans a reply or resolution out the same way
// post_review_comment fans out a new thread: the review-thread store does not
// broadcast on its own, so without this an agent's write is durably stored and
// invisible until the user remounts the review pane. canWriteReviewThread
// requires ThreadBroadcast to be non-nil before either tool is even registered,
// so — unlike a defensive nil check — calling it unconditionally here is the
// correct behavior: a reply or resolve that could not fan out must not exist as
// a tool at all, exactly like post_review_comment.
//
// The frame carries wsID's own project and repo, not the caller's: the thread
// this call just wrote to can be on a descendant or ancestor workspace of the
// caller's own (that is the whole point of the CanSee check above), and those
// can belong to a different repo when the caller sits at "home" — the /threads
// stream filters on projectId and repoId as well as wsId, so a frame carrying
// the caller's own repo would reach nobody's stream for a cross-repo write.
// wsID is already confirmed present in the visible set by the CanSee check that
// ran before every call site, so the lookup below always finds it — and the set
// is already loaded and memoised by that same check, which is why the load error
// is discarded here rather than failing the broadcast: a caller that reached this
// function loaded it successfully. The fallback to c.Workspace guards both a
// future call site that skips the check and the impossible-today error case,
// which broadcasts to the caller's own project/repo rather than not at all.
func broadcastThreadWrite(
	deps Deps,
	c Caller,
	wsID string,
	thread domain.ReviewThread,
) {
	ws := c.Workspace
	visible, _ := c.Visible()
	for _, w := range visible {
		if w.ID == wsID {
			ws = w
			break
		}
	}
	deps.ThreadBroadcast(thread.NormalizedMessages(), ws.ProjectID, ws.RepoID)
}
