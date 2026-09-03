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

// listReviewThreads renders ONE page of the caller's review threads, or — when
// threadId names one — that single thread with every message.
//
// threadId, offset and limit are the only arguments a model can widen anything
// with, and none reaches past the caller. The listing is keyed on
// c.Workspace.ID alone, so a page is a slice of what the caller could already
// see; threadId goes through the SAME CanSee check reply_to_review_thread and
// resolve_review_thread use, so it reaches exactly what those two can already
// write to and nothing more. Neither paging nor naming a thread may become a
// scope argument by the back door.
//
// The whole list is fetched and then sliced rather than pushed into the store's
// query, because filtering resolved threads out happens in Go — pushing an
// offset down to a store that has not applied that filter yet would page through
// the wrong list and skip threads.
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
	// Named-thread reads short-circuit BEFORE the listing, so they cost one row
	// rather than a whole workspace's threads — and so a thread on a workspace the
	// listing never covers is still reachable.
	if in.ThreadID != "" {
		return oneReviewThread(ctx, deps, c, in.ThreadID)
	}
	// c.Workspace.ID is the caller's OWN workspace, resolved by the Resolver
	// from its authenticated runner — the tool takes no workspace argument, so
	// there is no field here a model could steer elsewhere.
	threads, err := deps.Threads.ListByWorkspace(ctx, c.Workspace.ID)
	if err != nil {
		return "", fmt.Errorf("agenttools: list_review_threads: %w", err)
	}
	if !in.IncludeResolved {
		threads = unresolvedThreads(threads)
	}
	w := forwardWindow(len(threads), in.Offset, in.Limit, defaultThreadPage, maxThreadPage)
	note := forwardNote("threads", "list_review_threads", w)
	// A branch with no threads and an offset past the last page are different
	// answers, and only one of them means the review is clean. Rendering "No
	// review threads." for the second would tell a model there is nothing to
	// address at the exact moment it has paged past everything there is.
	if w.total == 0 {
		return render.RenderThreads(nil), nil
	}
	if w.empty() {
		return note, nil
	}
	return note + render.RenderThreads(threads[w.start:w.end]), nil
}

// oneReviewThread renders the named thread with every message it has, after the
// same authorization reply_to_review_thread and resolve_review_thread perform: a
// thread id names a thread in SOME workspace and authorizes nothing by itself, so
// the thread is looked up, its OWN WsID is checked against the caller's visible
// set, and only a thread that passes is rendered.
//
// CanSee — itself plus descendants — rather than "the caller's own workspace",
// deliberately, and it is the SAME rule the write tools already use. A surface
// where an agent may reply to a descendant's thread but may not read that thread
// in full is one an agent trips over: it answers a finding whose middle it was
// never able to see. The reachable set is therefore identical to what reply and
// resolve already reach, so this adds no exposure that surface did not have.
//
// The lead line states the message count for the same reason every other note
// does: a rendered thread and a COMPLETE rendered thread are the same bytes to a
// model unless the output says which it is holding.
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
	// Rejected BEFORE anything of the thread is rendered — the aggregate is in
	// hand at this point, and nothing but its WsID may be used until this passes.
	if !c.CanSee(thread.WsID) {
		return "", fmt.Errorf("agenttools: list_review_threads: %w", ErrOutOfScope)
	}
	// Resolved threads render too. includeResolved governs the LISTING; a model
	// that just resolved a thread and wants to re-read it should not also have to
	// know to flip a listing flag.
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
