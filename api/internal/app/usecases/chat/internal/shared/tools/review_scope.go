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

// getReviewScope reports the base ref, one page of changed files and their changed ranges, so a model
// has valid anchors before posting. The list is paginated, not just capped: a missing file reads as "not
// in this branch". Only rendering is bounded; GetScope still resolves the whole diff.
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
	// One resolution of the whole branch yields ref, files and geometry, so the ranges match the base.
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
