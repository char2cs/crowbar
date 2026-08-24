package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools/internal/render"
)

func getReviewScopeTool(deps Deps) toolDef {
	return toolDef{
		name: "get_review_scope",
		description: "What this branch review covers: base ref, changed files, and the changed line " +
			"ranges in each. Call before reviewing so findings target the right diff, and anchor " +
			"post_review_comment inside one of the ranges it reports.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"offset":{"type":"integer","minimum":0,"description":"Index of the first changed file to return. Defaults to 0; the reply says what to pass next."},
				"limit":{"type":"integer","minimum":1,"description":"How many changed files to return. Defaults to 100, capped at 300."}
			},
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			return getReviewScope(ctx, deps, c, args)
		},
	}
}

type getReviewScopeArgs struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// getReviewScope reports the review's base ref, ONE page of its changed files,
// and the changed line ranges of the files on that page.
//
// The ranges are here because this is the tool a model calls BEFORE it has an
// anchor. post_review_comment refuses an anchor that does not sit inside a
// changed hunk, and until this reply carried the geometry, nothing on the surface
// reported a line number at all — so the first anchor of every finding was a
// guess, and the only thing that ever told a model where the changed lines were
// was the refusal it got for guessing wrong. They come off the SAME outline that
// refusal is computed from, resolved in the same call (see gitdomain.ReviewScope),
// so this is not a second opinion about the diff that could disagree with the
// validator's.
//
// The file list is paginated rather than merely capped because it is what a
// model anchors comments against: post_review_comment rejects a path that is not
// in the review, so a file silently missing from this list reads as "not part of
// this branch" and the finding on it never gets written. A cap with no way past
// it would turn a large diff into a review of its first hundred files.
//
// Only the RENDERING is bounded. GetScope still resolves the whole branch diff,
// which is the same work whatever page is asked for — this cap is about the
// model's context, not about git.
func getReviewScope(
	ctx context.Context,
	deps Deps,
	c Caller,
	args json.RawMessage,
) (string, error) {
	var in getReviewScopeArgs
	if err := decode(args, &in); err != nil {
		return "", err
	}
	// GetScope is the whole BRANCH scope, never one commit against its parent —
	// and it reports the ref, the files and the geometry from a single
	// resolution, so the ranges it renders are guaranteed to be ranges of the
	// diff of the base it names.
	scope, err := deps.Review.GetScope(ctx, c.Workspace)
	if err != nil {
		return "", fmt.Errorf("agenttools: get_review_scope: %w", err)
	}
	w := forwardWindow(len(scope.Files), in.Offset, in.Limit, defaultScopeFiles, maxScopeFiles)
	note := forwardNote("changed files", "get_review_scope", w)
	if w.empty() {
		return render.RenderScope(scope.Base, nil, nil, w.start, note), nil
	}
	return render.RenderScope(scope.Base, scope.Files[w.start:w.end], scope.Outline, w.start, note), nil
}
