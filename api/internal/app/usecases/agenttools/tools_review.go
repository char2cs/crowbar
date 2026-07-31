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

// ReviewReader is the narrow read port the review tools need from the
// branch-review usecase: the scope of a review — the ref it diffs against
// together with the file-level change summary — and (Task 12) the hunk geometry
// of that same diff.
//
// Scope is one method rather than a base getter beside a file getter because the
// two were resolving the same ref independently, at up to three git subprocesses
// each, to serve a single tool call. See gitdomain.ReviewScope.
//
// It takes the caller's resolved workspace, not its id: the Resolver has already
// folded that aggregate to authenticate the call, and re-reading it replays the
// event log and fires a second background reconcile for an answer the tool is
// already holding.
type ReviewReader interface {
	GetScope(ctx context.Context, ws domain.Workspace) (gitdomain.ReviewScope, error)
	GetOutline(ctx context.Context, wsID, commit string) ([]gitdomain.FileOutline, error)
}

// ThreadReader is the narrow read port the review tools need from the
// review-thread store: ListByWorkspace for list_review_threads' listing, and Get
// for every tool that acts on ONE named thread — reply_to_review_thread,
// resolve_review_thread, and list_review_threads' own threadId — each of which
// must look a thread up before touching it, to learn which workspace it actually
// belongs to. A thread id names a thread in SOME workspace; it is not itself an
// authorization, so the WsID that Get returns is what CanSee is checked
// against, never the id argument on its own.
type ThreadReader interface {
	ListByWorkspace(ctx context.Context, wsID string) ([]domain.ReviewThread, error)
	Get(ctx context.Context, id string) (domain.ReviewThread, error)
}

// ThreadBroadcast announces a newly written review thread to connected clients.
//
// It exists because the review-thread repository does NOT fan out: its
// store.BroadcastFunc is wired to a no-op, and the only producer of /threads
// WebSocket frames is the HTTP handler pushing a DTO it built from the request
// path. An agent write bypasses that handler entirely, so without this port a
// posted finding is durably stored and completely invisible until the user
// remounts the review pane — which defeats the point of the tool.
//
// It takes the DOMAIN aggregate plus the owning project and repo ids, never a
// wire DTO: the /threads stream filters on projectId, repoId and wsId, and the
// aggregate carries only WsID, so the ids have to come from the caller's resolved
// workspace. Converting to the DTO is the wiring layer's job — a usecase must not
// import the api layer's wire types.
type ThreadBroadcast func(
	thread domain.ReviewThread,
	projectID string,
	repoID string,
)

// ThreadWriter is the write half of the review-thread port: opening a thread,
// replying to one, and resolving it. Task 12 is the first to implement and call
// it; it is declared here so Deps.ThreadWrites has a stable shape across both
// tasks, matching the reviewthread store's own Open/Reply/Resolve signatures so
// that store can satisfy this port directly, with no adapter.
type ThreadWriter interface {
	Open(ctx context.Context, in reviewthread.OpenInput, now time.Time) (domain.ReviewThread, error)
	Reply(
		ctx context.Context,
		in reviewthread.ReplyInput,
		now time.Time,
	) (domain.ReviewThread, error)
	Resolve(ctx context.Context, id string) (domain.ReviewThread, error)
}

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
		return renderThreads(nil), nil
	}
	if w.empty() {
		return note, nil
	}
	return note + renderThreads(threads[w.start:w.end]), nil
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
	) + renderThread(thread), nil
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

// getReviewScope reports the review's base ref and ONE page of its changed
// files.
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
	// and it reports the ref and the files from a single resolution, so the file
	// list it renders is guaranteed to be the diff of the base it names.
	scope, err := deps.Review.GetScope(ctx, c.Workspace)
	if err != nil {
		return "", fmt.Errorf("agenttools: get_review_scope: %w", err)
	}
	w := forwardWindow(len(scope.Files), in.Offset, in.Limit, defaultScopeFiles, maxScopeFiles)
	note := forwardNote("changed files", "get_review_scope", w)
	if w.empty() {
		return renderScope(scope.Base, nil, note), nil
	}
	return renderScope(scope.Base, scope.Files[w.start:w.end], note), nil
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
	if err := validateAnchor(outline, in.FilePath, in.StartLine, in.EndLine, side); err != nil {
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
// Where it is actually read today is the AGENT-facing view: renderThreads prints
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

func replyToReviewThreadTool(deps Deps) toolDef {
	return toolDef{
		name:        "reply_to_review_thread",
		description: "Reply to an existing review thread. Get thread ids from list_review_threads.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"threadId":{"type":"string","description":"The thread to reply to. Get ids from list_review_threads."},
				"body":{"type":"string","description":"The reply, in markdown."}
			},
			"required":["threadId","body"],
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			return replyToReviewThread(ctx, deps, c, args)
		},
	}
}

type replyToReviewThreadArgs struct {
	ThreadID string `json:"threadId"`
	Body     string `json:"body"`
}

// replyToReviewThread looks the thread up before writing to it: a thread id
// names a thread in SOME workspace, so the id by itself authorizes nothing. Only
// once the thread's actual WsID is known and confirmed visible to the caller
// does the reply reach the store — a rejection here must never call
// deps.ThreadWrites.Reply at all.
func replyToReviewThread(
	ctx context.Context,
	deps Deps,
	c Caller,
	args json.RawMessage,
) (string, error) {
	var in replyToReviewThreadArgs
	if err := decode(args, &in); err != nil {
		return "", err
	}
	if strings.TrimSpace(in.Body) == "" {
		return "", fmt.Errorf("agenttools: reply_to_review_thread: body must not be empty")
	}
	thread, err := deps.Threads.Get(ctx, in.ThreadID)
	if err != nil {
		return "", fmt.Errorf("agenttools: reply_to_review_thread: %w", err)
	}
	if !c.CanSee(thread.WsID) {
		return "", fmt.Errorf("agenttools: reply_to_review_thread: %w", ErrOutOfScope)
	}
	updated, err := deps.ThreadWrites.Reply(ctx, replyInputFor(c, in.ThreadID, in.Body), time.Now())
	if err != nil {
		return "", fmt.Errorf("agenttools: reply_to_review_thread: %w", err)
	}
	broadcastThreadWrite(deps, c, thread.WsID, updated)
	return fmt.Sprintf("Replied to review thread %s.", in.ThreadID), nil
}

// replyInputFor builds the reply write, attributing it exactly the way
// openInputFor attributes a new thread: provider and chat come off the resolved
// Caller and never off the tool's arguments, so a model cannot file a reply under
// another agent's name or another conversation.
//
// The thread this reply lands on may belong to a workspace other than the
// caller's own — CanSee spans descendants — so the attribution deliberately
// describes the WRITER, not the thread's workspace.
func replyInputFor(
	c Caller,
	threadID string,
	body string,
) reviewthread.ReplyInput {
	return reviewthread.ReplyInput{
		ID:         threadID,
		MessageID:  uuid.NewString(),
		Author:     authorOf(c),
		IsAgent:    true,
		ProviderID: c.ProviderID,
		ChatID:     c.ChatID,
		Body:       body,
	}
}

func resolveReviewThreadTool(deps Deps) toolDef {
	return toolDef{
		name:        "resolve_review_thread",
		description: "Mark a review thread resolved. Only resolve a thread whose finding you have actually addressed.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"threadId":{"type":"string","description":"The thread to resolve. Get ids from list_review_threads."}
			},
			"required":["threadId"],
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			return resolveReviewThread(ctx, deps, c, args)
		},
	}
}

type resolveReviewThreadArgs struct {
	ThreadID string `json:"threadId"`
}

// resolveReviewThread carries the same scope check as replyToReviewThread, and
// for the same reason: the thread id alone says nothing about who may act on it,
// only the thread's own WsID does.
func resolveReviewThread(
	ctx context.Context,
	deps Deps,
	c Caller,
	args json.RawMessage,
) (string, error) {
	var in resolveReviewThreadArgs
	if err := decode(args, &in); err != nil {
		return "", err
	}
	thread, err := deps.Threads.Get(ctx, in.ThreadID)
	if err != nil {
		return "", fmt.Errorf("agenttools: resolve_review_thread: %w", err)
	}
	if !c.CanSee(thread.WsID) {
		return "", fmt.Errorf("agenttools: resolve_review_thread: %w", ErrOutOfScope)
	}
	updated, err := deps.ThreadWrites.Resolve(ctx, in.ThreadID)
	if err != nil {
		return "", fmt.Errorf("agenttools: resolve_review_thread: %w", err)
	}
	broadcastThreadWrite(deps, c, thread.WsID, updated)
	return fmt.Sprintf("Resolved review thread %s.", in.ThreadID), nil
}

// broadcastThreadWrite fans a reply or resolution out the same way
// post_review_comment fans out a new thread: the review-thread store does not
// broadcast on its own, so without this an agent's write is durably stored and
// invisible until the user remounts the review pane. canWriteReviewThread
// requires ThreadBroadcast to be non-nil before either tool is even registered,
// so — unlike a defensive nil check — calling it unconditionally here is the
// correct behavior: a reply or resolve that could not fan out must not exist as
// a tool at all, exactly like post_review_comment.
//
// The frame carries wsID's own project and repo, not the caller's: the thread
// this call just wrote to can be on a descendant or ancestor workspace of the
// caller's own (that is the whole point of the CanSee check above), and those
// can belong to a different repo when the caller sits at "home" — the /threads
// stream filters on projectId and repoId as well as wsId, so a frame carrying
// the caller's own repo would reach nobody's stream for a cross-repo write.
// wsID is already confirmed present in the visible set by the CanSee check that
// ran before every call site, so the lookup below always finds it — and the set
// is already loaded and memoised by that same check, which is why the load error
// is discarded here rather than failing the broadcast: a caller that reached this
// function loaded it successfully. The fallback to c.Workspace guards both a
// future call site that skips the check and the impossible-today error case,
// which broadcasts to the caller's own project/repo rather than not at all.
func broadcastThreadWrite(
	deps Deps,
	c Caller,
	wsID string,
	thread domain.ReviewThread,
) {
	ws := c.Workspace
	visible, _ := c.Visible()
	for _, w := range visible {
		if w.ID == wsID {
			ws = w
			break
		}
	}
	deps.ThreadBroadcast(thread.NormalizedMessages(), ws.ProjectID, ws.RepoID)
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
// A binary file is rejected by the hunk loop finding nothing to match: git emits no
// `@@` headers for one, so its Hunks slice is empty.
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

// anchorInAnyHunk requires the WHOLE range to fall inside one hunk. lo+span-1 is
// the hunk's last line, so a hunk at NewStart 40 spanning 10 lines covers 40..49
// inclusive: 39 and 50 are outside it. A side that is absent (`@@ -1,2 +0,0 @@`
// gives span 0) has an empty range and matches nothing, since checkAnchorRange has
// already established start <= end.
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
		if start >= lo && end <= lo+span-1 {
			return true
		}
	}
	return false
}
