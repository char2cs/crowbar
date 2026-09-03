package tools

import (
	"context"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The four seams the review tools read and write the code-review record through.

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
