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

// postReviewComment validates the anchor against the CURRENT review before it
// writes anything, then opens the thread under the caller's own workspace.
//
// The order matters: the validation is the only thing standing between a model's
// guessed line number and a stored comment the user cannot interpret, so it runs
// before the write and a failure returns without touching the store.
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
	// commit="" is the whole BRANCH scope — the same diff get_review_scope reports,
	// and the widest one, so an anchor accepted here is in the branch diff. A user
	// viewing a single commit (the review routes take ?sha=) sees narrower geometry,
	// so a branch-valid anchor can still fall outside the view they happen to be on.
	// c.Workspace.ID is the caller's OWN workspace, resolved from its runner: the
	// anchor is checked against the diff the comment will be attached to, never
	// another workspace's.
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
	// Only a real write is announced, and it is announced with the aggregate the store
	// just returned. A dedup hit changed nothing, so a frame for it would tell every
	// connected client to re-render a thread it already has.
	if out.created {
		deps.ThreadBroadcast(out.fresh.NormalizedMessages(), c.Workspace.ProjectID, c.Workspace.RepoID)
	}
	// The anchor is read back off the STORED thread, not off the arguments: a retry
	// that reuses a key with different lines wrote nothing, and echoing its own
	// arguments would report a comment at a location no thread is anchored to.
	stored := out.stored
	return fmt.Sprintf(
		"Posted review comment %s on %s:%d-%d (%s side).",
		stored.ID, stored.FilePath, stored.StartLine, stored.EndLine, stored.Side,
	), nil
}

// openInputFor builds the thread write. WsID is the caller's OWN workspace as
// resolved from its runner — the tool takes no workspace argument, so there is no
// field a model could steer at somebody else's review.
//
// LineNumber repeats StartLine: it is the aggregate's original single-line anchor,
// still read by clients that predate ranges, and leaving it zero would render the
// comment against line 0.
//
// ProviderID and ChatID come off the resolved Caller, never off the arguments, for
// the same reason WsID does: attribution a model could type is attribution a model
// could forge, and a finding filed under another agent's name is worse than an
// anonymous one. ChatID is the runner's CURRENT chat, so it names the conversation
// the finding was actually reasoned in.
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

// authorOf attributes the finding to the vendor CLI that reported it.
//
// Where it is actually read today is the AGENT-facing view: render.RenderThreads prints
// "<author> (agent)", so a second agent addressing review comments can tell which
// CLI left which finding. The review UI currently labels every isAgent message
// "Agent" and discards the author, so this does not yet reach the user's eye —
// storing it is what makes attributing them possible without a data migration.
//
// A runner with no provider recorded still gets a non-empty author, because a
// blank one renders as an unnamed speaker.
func authorOf(c Caller) string {
	if c.ProviderID == "" {
		return "agent"
	}
	return c.ProviderID
}
