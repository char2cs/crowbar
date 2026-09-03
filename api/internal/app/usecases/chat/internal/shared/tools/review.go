package tools

// reviewTools registers the review tools, each independently fail-closed on its
// own dependencies: a nil port means the tool that needs it is simply not
// advertised.
//
// post_review_comment needs four of them, and not one is optional. Review: a
// comment is only posted after its anchor is checked against the diff geometry, so
// with no outline reader the tool must not exist at all rather than write
// unvalidated anchors. Idempotency: without it a retried post silently duplicates
// a finding. ThreadBroadcast: without it the comment never reaches an open review
// pane, and a tool whose whole purpose is to put a finding in front of the user
// is worse than absent if it writes somewhere the user is not looking.
//
// reply_to_review_thread and resolve_review_thread need a thread reader (to look
// up which workspace a thread belongs to before writing), the writer, and
// ThreadBroadcast for the same reason post_review_comment does: an agent's
// reply or resolution bypasses the HTTP handler that normally pushes a /threads
// frame, so without a broadcaster the write is stored and silently invisible to
// an open review pane. A tool that only sometimes updates what the user is
// looking at is worse than one that plainly does not exist.
//
// Neither of those two is gated on Idempotency, unlike post_review_comment, and
// the difference is deliberate. They register as a PAIR, and resolve_review_thread
// is idempotent by nature — resolving twice leaves the same thread resolved — so
// requiring a dedup map for the pair would suppress a tool that has no use for
// one. reply_to_review_thread stays useful without a map too: its key is optional,
// so only a call that actually supplied one is refused, and it is refused rather
// than written unguarded (see errNoDedupMap).
func reviewTools(deps Deps) []toolDef {
	var out []toolDef
	if deps.Threads != nil {
		out = append(out, listReviewThreadsTool(deps))
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
