package tree

import (
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// The placement refusals are the Tree's own, one code per rule (domain/tree).
var (
	ErrCycle          = dtree.ErrCycle
	ErrCrossWorkspace = dtree.ErrCrossWorkspace
	ErrCrossContext   = dtree.ErrCrossContext
	ErrForkChain      = dtree.ErrForkChain
	ErrNotAContainer  = dtree.ErrNotAContainer
	ErrNameRequired   = dtree.ErrNameRequired
	ErrSubtreeWorking = dtree.ErrSubtreeWorking
)

// ErrWorkspaceUnprovisioned is returned when a create names a workspace that
// has no worktree on disk — a PLACEHOLDER (hierarchy.ErrParentUnprovisioned's
// own subject, seen from the create side): a branch another checkout holds, or
// one whose materialisation failed. The chat would have nowhere to run, and
// the failure surfaced nowhere: the create answered 201, the runner spawned,
// the sidebar drew an ordinary new row and pane, and only the daemon log knew
// ("agents: catalog worktree is invalid" on every read of it). Refused before
// anything is minted, so the create-row error path shows the real reason.
// Handlers map it to 409 — the row is not in a state where this can be done,
// not a malformed request.
var ErrWorkspaceUnprovisioned = problem.New(problem.Conflict, "usecases: this branch has no worktree yet")

// ErrNowhereToRun is a plain chat create naming neither a workspace nor a row
// to run under: refused before anything is minted.
var ErrNowhereToRun = problem.New(problem.InvalidArgument, "A new chat needs a workspace or a row to run under")
