package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools/internal/render"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func postReviewCommentTool(deps Deps) toolDef {
	return toolDef{
		name: "post_review_comment",
		description: "Post a review finding as a thread anchored to a file and line range, " +
			"visible in Crowbar's review UI. Use this instead of writing findings in chat " +
			"when reviewing a branch.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"filePath":{"type":"string","description":"Path as it appears in the review's changed-file list."},
				"startLine":{"type":"integer","description":"First line of the anchor, in the numbering of the chosen side."},
				"endLine":{"type":"integer","description":"Last line of the anchor. Same as startLine for a single line."},
				"side":{"type":"string","enum":["left","right"],"description":"left = the base revision, right = this branch. Use right unless commenting on removed code."},
				"body":{"type":"string","description":"The finding, in markdown."},
				"idempotencyKey":{"type":"string","description":"Stable key for this finding; a retry with the same key will not duplicate the comment."}
			},
			"required":["filePath","startLine","endLine","side","body"],
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			return postReviewComment(ctx, deps, c, args)
		},
	}
}

type postReviewCommentArgs struct {
	FilePath       string `json:"filePath"`
	StartLine      int    `json:"startLine"`
	EndLine        int    `json:"endLine"`
	Side           string `json:"side"`
	Body           string `json:"body"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// postReviewComment validates the anchor against the current review before writing anything, then opens
// the thread under the caller's own workspace; a guessed line must never become a stored comment.
func postReviewComment(
	ctx context.Context,
	deps Deps,
	c Caller,
	args json.RawMessage,
) (string, error) {
	var in postReviewCommentArgs
	if err := decode(args, &in); err != nil {
		return "", err
	}
	side, err := render.ParseSide(in.Side)
	if err != nil {
		return "", err
	}
	if err := render.CheckRange(in.StartLine, in.EndLine); err != nil {
		return "", err
	}
	if err := render.CheckBody("post_review_comment", in.Body); err != nil {
		return "", err
	}
	// commit="" is the whole branch diff get_review_scope reports; a user viewing one commit sees
	// narrower geometry, so a branch-valid anchor can still fall outside their view.
	outline, err := deps.Review.GetOutline(ctx, c.Workspace.ID, "")
	if err != nil {
		return "", fmt.Errorf("agenttools: post_review_comment: outline: %w", err)
	}
	if err := render.Validate(outline, in.FilePath, in.StartLine, in.EndLine, side); err != nil {
		return "", err
	}
	out, err := deps.Idempotency.openOnce(
		ctx,
		deps.ThreadWrites,
		in.IdempotencyKey,
		openInputFor(c, in, side),
		time.Now(),
	)
	if err != nil {
		return "", fmt.Errorf("agenttools: post_review_comment: %w", err)
	}
	// Only a real write is announced; a dedup hit changed nothing.
	if out.created {
		deps.ThreadBroadcast(out.fresh.NormalizedMessages(), c.Workspace.ProjectID, c.Workspace.RepoID)
	}
	// Report the stored anchor: a keyed retry with different lines wrote nothing.
	stored := out.stored
	return fmt.Sprintf(
		"Posted review comment %s on %s:%d-%d (%s side).",
		stored.ID, stored.FilePath, stored.StartLine, stored.EndLine, stored.Side,
	), nil
}

// openInputFor builds the thread write. WsID, ProviderID and ChatID come from the resolved Caller, never
// the arguments, so a model cannot steer or forge them. LineNumber repeats StartLine for clients that
// read only the single-line anchor.
func openInputFor(
	c Caller,
	in postReviewCommentArgs,
	side domain.ReviewSide,
) reviewthread.OpenInput {
	return reviewthread.OpenInput{
		ID:         uuid.NewString(),
		WsID:       c.Workspace.ID,
		FilePath:   in.FilePath,
		LineNumber: in.StartLine,
		StartLine:  in.StartLine,
		EndLine:    in.EndLine,
		Side:       side,
		MessageID:  uuid.NewString(),
		Author:     authorOf(c),
		IsAgent:    true,
		ProviderID: c.ProviderID,
		ChatID:     c.ChatID,
		Body:       in.Body,
	}
}

// authorOf attributes the finding to the vendor CLI that reported it, so another agent can tell who left
// it. It is never empty: a blank author renders as the user.
func authorOf(c Caller) string {
	if c.ProviderID == "" {
		return "agent"
	}
	return c.ProviderID
}
