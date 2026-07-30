package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ReviewReader is the narrow read port the review-scope tool needs from the
// branch-review usecase: the ref a review diffs against, the file-level change
// summary, and (Task 12) the hunk geometry of that same diff.
type ReviewReader interface {
	GetBase(ctx context.Context, wsID string) (string, error)
	GetFiles(ctx context.Context, wsID string, commit string) ([]gitdomain.ReviewFileSummary, error)
	GetOutline(ctx context.Context, wsID string, commit string) ([]gitdomain.FileOutline, error)
}

// ThreadReader is the narrow read port list_review_threads needs from the
// review-thread store.
type ThreadReader interface {
	ListByWorkspace(ctx context.Context, wsID string) ([]domain.ReviewThread, error)
	Get(ctx context.Context, id string) (domain.ReviewThread, error)
}

// ThreadWriter is the write half of the review-thread port: opening a thread,
// replying to one, and resolving it. Task 12 is the first to implement and call
// it; it is declared here so Deps.ThreadWrites has a stable shape across both
// tasks, matching the reviewthread store's own Open/Reply/Resolve signatures so
// that store can satisfy this port directly, with no adapter.
type ThreadWriter interface {
	Open(ctx context.Context, in reviewthread.OpenInput, now time.Time) (domain.ReviewThread, error)
	Reply(
		ctx context.Context,
		id string,
		messageID string,
		author string,
		isAgent bool,
		body string,
		now time.Time,
	) (domain.ReviewThread, error)
	Resolve(ctx context.Context, id string) (domain.ReviewThread, error)
}

// reviewTools registers the two read-only review tools, each independently
// fail-closed on its own dependency: a nil Threads or Review means the tool
// that needs it is simply not advertised.
func reviewTools(deps Deps) []toolDef {
	var out []toolDef
	if deps.Threads != nil {
		out = append(out, listReviewThreadsTool(deps))
	}
	if deps.Review != nil {
		out = append(out, getReviewScopeTool(deps))
	}
	return out
}

func listReviewThreadsTool(deps Deps) toolDef {
	return toolDef{
		name:        "list_review_threads",
		description: "List the code-review threads the user left on this branch. Call when asked to address, answer, or check review comments.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"includeResolved":{"type":"boolean","description":"Include already-resolved threads. Defaults to false (unresolved only)."}
			},
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			var in struct {
				IncludeResolved bool `json:"includeResolved"`
			}
			if err := decode(args, &in); err != nil {
				return "", err
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
			return renderThreads(threads), nil
		},
	}
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

func getReviewScopeTool(deps Deps) toolDef {
	return toolDef{
		name:        "get_review_scope",
		description: "What this branch review covers: base ref and changed files. Call before reviewing so findings target the right diff.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{},
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, _ json.RawMessage) (string, error) {
			// commit="" means the whole branch scope, not one commit against its
			// parent — see ReviewReader.GetFiles.
			base, err := deps.Review.GetBase(ctx, c.Workspace.ID)
			if err != nil {
				return "", fmt.Errorf("agenttools: get_review_scope: base: %w", err)
			}
			files, err := deps.Review.GetFiles(ctx, c.Workspace.ID, "")
			if err != nil {
				return "", fmt.Errorf("agenttools: get_review_scope: files: %w", err)
			}
			return renderScope(base, files), nil
		},
	}
}
