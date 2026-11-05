package tools

// reviewTools registers the review tools, each fail-closed on its own ports: a write tool without the
// diff outline, writer or ThreadBroadcast is not advertised, since an unvalidated or invisible write is
// worse than none. Only post_review_comment also needs Idempotency; reply/resolve refuse a keyed call.
func reviewTools(deps Deps) []toolDef {
	var out []toolDef
	if deps.Threads != nil {
		out = append(out, listReviewThreadsTool(deps))
	}
	if deps.Threads != nil && deps.ThreadChanges != nil {
		out = append(out, waitForReviewCommentsTool(deps))
	}
	if deps.Review != nil {
		out = append(out, getReviewScopeTool(deps))
	}
	if canPostReviewComment(deps) {
		out = append(out, postReviewCommentTool(deps))
	}
	if canWriteReviewThread(deps) {
		out = append(out, replyToReviewThreadTool(deps))
		out = append(out, resolveReviewThreadTool(deps))
	}
	return out
}

func canPostReviewComment(deps Deps) bool {
	return deps.Review != nil &&
		deps.ThreadWrites != nil &&
		deps.Idempotency != nil &&
		deps.ThreadBroadcast != nil
}

func canWriteReviewThread(deps Deps) bool {
	return deps.Threads != nil && deps.ThreadWrites != nil && deps.ThreadBroadcast != nil
}
