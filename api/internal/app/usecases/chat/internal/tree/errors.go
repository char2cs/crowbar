package tree

import (
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

var (
	ErrCycle          = dtree.ErrCycle
	ErrCrossWorkspace = dtree.ErrCrossWorkspace
	ErrCrossContext   = dtree.ErrCrossContext
	ErrForkChain      = dtree.ErrForkChain
	ErrNotAContainer  = dtree.ErrNotAContainer
	ErrNameRequired   = dtree.ErrNameRequired
	ErrSubtreeWorking = dtree.ErrSubtreeWorking
)

var ErrWorkspaceUnprovisioned = problem.New(problem.Conflict, "usecases: this branch has no worktree yet")

var ErrNowhereToRun = problem.New(problem.InvalidArgument, "A new chat needs a workspace or a row to run under")

var ErrThreadNeedsWorkspace = problem.New(problem.InvalidArgument,
	"A thread runs in its chat's workspace: name it in workspaceId")

var ErrForkNeedsProvider = problem.New(problem.InvalidArgument, "A chat on a new branch needs a provider to start")
