package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ReviewReader is the narrow read port the review-scope tool needs from the
// branch-review usecase: the ref a review diffs against, the file-level change
// summary, and (Task 12) the hunk geometry of that same diff.
type ReviewReader interface {
	GetBase(ctx context.Context, wsID string) (string, error)
	GetFiles(ctx context.Context, wsID, commit string) ([]gitdomain.ReviewFileSummary, error)
	GetOutline(ctx context.Context, wsID, commit string) ([]gitdomain.FileOutline, error)
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

// reviewTools registers the review tools, each independently fail-closed on its
// own dependencies: a nil port means the tool that needs it is simply not
// advertised.
//
// post_review_comment needs three of them. Review is not optional for it — a
// comment is only posted after its anchor is checked against the diff geometry,
// so with no outline reader the tool must not exist at all rather than write
// unvalidated anchors. Idempotency is not optional either: without it a retried
// post would silently duplicate a finding.
func reviewTools(deps Deps) []toolDef {
	var out []toolDef
	if deps.Threads != nil {
		out = append(out, listReviewThreadsTool(deps))
	}
	if deps.Review != nil {
		out = append(out, getReviewScopeTool(deps))
	}
	if deps.Review != nil && deps.ThreadWrites != nil && deps.Idempotency != nil {
		out = append(out, postReviewCommentTool(deps))
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
	side, err := parseSide(in.Side)
	if err != nil {
		return "", err
	}
	if err := checkAnchorRange(in.StartLine, in.EndLine); err != nil {
		return "", err
	}
	if strings.TrimSpace(in.Body) == "" {
		return "", fmt.Errorf("agenttools: post_review_comment: body must not be empty")
	}
	// commit="" is the whole branch scope, the same diff get_review_scope reports
	// and the same one the review UI renders — so an anchor accepted here is an
	// anchor the user will actually see code beside.
	outline, err := deps.Review.GetOutline(ctx, c.Workspace.ID, "")
	if err != nil {
		return "", fmt.Errorf("agenttools: post_review_comment: outline: %w", err)
	}
	if err := validateAnchor(outline, in.FilePath, in.StartLine, in.EndLine, side); err != nil {
		return "", err
	}
	id, err := deps.Idempotency.OpenOnce(
		ctx,
		deps.ThreadWrites,
		in.IdempotencyKey,
		openInputFor(c, in, side),
		time.Now(),
	)
	if err != nil {
		return "", fmt.Errorf("agenttools: post_review_comment: %w", err)
	}
	return fmt.Sprintf(
		"Posted review comment %s on %s:%d-%d (%s side).",
		id, in.FilePath, in.StartLine, in.EndLine, side,
	), nil
}

// openInputFor builds the thread write. WsID is the caller's OWN workspace as
// resolved from its runner — the tool takes no workspace argument, so there is no
// field a model could steer at somebody else's review.
//
// LineNumber repeats StartLine: it is the aggregate's original single-line anchor,
// still read by clients that predate ranges, and leaving it zero would render the
// comment against line 0.
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
		Body:       in.Body,
	}
}

// authorOf attributes the finding to the vendor CLI that reported it, so the
// review UI can label it beside the user's own comments. A runner with no
// provider recorded still gets a non-empty author, because a blank one renders as
// an unnamed speaker in the thread.
func authorOf(c Caller) string {
	if c.ProviderID == "" {
		return "agent"
	}
	return c.ProviderID
}

func parseSide(side string) (domain.ReviewSide, error) {
	switch domain.ReviewSide(side) {
	case domain.ReviewSideLeft:
		return domain.ReviewSideLeft, nil
	case domain.ReviewSideRight:
		return domain.ReviewSideRight, nil
	}
	return "", fmt.Errorf("agenttools: post_review_comment: side must be \"left\" or \"right\", got %q", side)
}

func checkAnchorRange(start, end int) error {
	if start < 1 || end < start {
		return fmt.Errorf(
			"agenttools: post_review_comment: startLine %d and endLine %d are not a line range; "+
				"lines are 1-based and endLine must not precede startLine",
			start, end,
		)
	}
	return nil
}

// validateAnchor rejects an anchor that does not land in a hunk of the CURRENT
// review rather than storing it: a floating comment is worse than no comment,
// because the user sees a finding with no code next to it and cannot tell what it
// refers to.
//
// The side selects which pair of the hunk's four numbers applies. right is the
// branch (NewStart/NewLines), left the base revision (OldStart/OldLines) — a
// deleted line only exists on the left, and the two numberings diverge by every
// insertion above them, so validating against the wrong pair would accept anchors
// that render nowhere.
//
// A file matches on either path because a rename carries both: Path is the new
// name the review addresses it by, OldPath the name it had on the base side.
//
// A file whose outline is partial (its hunk count ran past the collection cap) can
// still refuse a legitimate anchor past the cap. That is the safe direction: the
// model is told to re-read the scope, rather than the user being shown a comment
// against nothing.
func validateAnchor(
	outline []gitdomain.FileOutline,
	path string,
	start, end int,
	side domain.ReviewSide,
) error {
	for _, f := range outline {
		if f.Path != path && f.OldPath != path {
			continue
		}
		if anchorInAnyHunk(f.Hunks, start, end, side) {
			return nil
		}
		return fmt.Errorf(
			"agenttools: lines %d-%d of %s are not in any changed hunk on the %s side; "+
				"call get_review_scope and anchor inside a changed range",
			start, end, path, side,
		)
	}
	return fmt.Errorf(
		"agenttools: %s is not part of this review; call get_review_scope for the changed files",
		path,
	)
}

func anchorInAnyHunk(
	hunks []gitdomain.HunkShape,
	start, end int,
	side domain.ReviewSide,
) bool {
	for _, h := range hunks {
		lo, span := h.NewStart, h.NewLines
		if side == domain.ReviewSideLeft {
			lo, span = h.OldStart, h.OldLines
		}
		if span > 0 && start >= lo && end <= lo+span-1 {
			return true
		}
	}
	return false
}
