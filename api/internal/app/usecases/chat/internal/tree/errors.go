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

// ErrWorkspaceUnprovisioned is returned when a create names a placeholder workspace with no worktree on
// disk (held by another checkout, or failed to materialise): the chat would have nowhere to run, so it is
// refused before anything is minted. Handlers map it to 409.
var ErrWorkspaceUnprovisioned = problem.New(problem.Conflict, "usecases: this branch has no worktree yet")

// ErrNowhereToRun is a plain chat create naming neither a workspace nor a row
// to run under: refused before anything is minted.
var ErrNowhereToRun = problem.New(problem.InvalidArgument, "A new chat needs a workspace or a row to run under")

// ErrForkNeedsProvider is a fork create naming no provider: it starts a CLI in
// the worktree it cuts, so it is refused before anything is minted.
var ErrForkNeedsProvider = problem.New(problem.InvalidArgument, "A chat on a new branch needs a provider to start")
