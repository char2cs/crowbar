package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools/internal/render"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func listReviewThreadsTool(deps Deps) toolDef {
	return toolDef{
		name:        "list_review_threads",
		description: "List the code-review threads the user left on this branch. Call when asked to address, answer, or check review comments.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"threadId":{"type":"string","description":"Render just this one thread, with every message it has. Use it when a listing said replies were not shown. Ids come from this tool's own listing."},
				"includeResolved":{"type":"boolean","description":"Include already-resolved threads. Defaults to false (unresolved only)."},
				"offset":{"type":"integer","minimum":0,"description":"Index of the first thread to return. Defaults to 0; the reply says what to pass next."},
				"limit":{"type":"integer","minimum":1,"description":"How many threads to return. Defaults to 20, capped at 50."}
			},
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			return listReviewThreads(ctx, deps, c, args)
		},
	}
}

type listReviewThreadsArgs struct {
	ThreadID        string `json:"threadId"`
	IncludeResolved bool   `json:"includeResolved"`
	Offset          int    `json:"offset"`
	Limit           int    `json:"limit"`
}

// listReviewThreads renders one page of the caller's own workspace's threads, or the single thread
// threadId names (checked with the write tools' CanSee rule). The list is sliced in Go because the
// resolved filter runs here; a store-side offset would page the wrong list.
func listReviewThreads(
	ctx context.Context,
	deps Deps,
	c Caller,
	args json.RawMessage,
) (string, error) {
	var in listReviewThreadsArgs
	if err := decode(args, &in); err != nil {
		return "", err
	}
	// A named thread costs one row, and may live on a descendant the listing does not cover.
	if in.ThreadID != "" {
		return oneReviewThread(ctx, deps, c, in.ThreadID)
	}
	// c.Workspace.ID comes from the authenticated runner; no argument can steer it.
	threads, err := deps.Threads.ListByWorkspace(ctx, c.Workspace.ID)
	if err != nil {
		return "", fmt.Errorf("agenttools: list_review_threads: %w", err)
	}
	if !in.IncludeResolved {
		threads = unresolvedThreads(threads)
	}
	w := forwardWindow(len(threads), in.Offset, in.Limit, defaultThreadPage, maxThreadPage)
	note := forwardNote("threads", "list_review_threads", w)
	// Only an empty branch means the review is clean; past the last page, the note explains.
	if w.total == 0 {
		return render.RenderThreads(nil), nil
	}
	if w.empty() {
		return note, nil
	}
	return note + render.RenderThreads(threads[w.start:w.end]), nil
}

// oneReviewThread renders the named thread in full if its own WsID passes CanSee, the same rule the
// reply and resolve tools use, so an agent can read every thread it may answer and no more.
func oneReviewThread(
	ctx context.Context,
	deps Deps,
	c Caller,
	threadID string,
) (string, error) {
	thread, err := deps.Threads.Get(ctx, threadID)
	if err != nil {
		return "", fmt.Errorf("agenttools: list_review_threads: %w", err)
	}
	// Nothing but WsID may be used before this check passes.
	if !c.CanSee(thread.WsID) {
		return "", fmt.Errorf("agenttools: list_review_threads: %w", ErrOutOfScope)
	}
	// Resolved threads render too: includeResolved governs only the listing.
	return fmt.Sprintf(
		"Showing thread %s in full: %d messages.\n", thread.ID, len(thread.Messages),
	) + render.RenderThread(thread), nil
}

func unresolvedThreads(threads []domain.ReviewThread) []domain.ReviewThread {
	out := make([]domain.ReviewThread, 0, len(threads))
	for _, t := range threads {
		if !t.IsResolved() {
			out = append(out, t)
		}
	}
	return out
}
