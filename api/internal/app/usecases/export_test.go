package usecases

import (
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
)

// NewAgentToolDepsForTest exposes newAgentToolDeps so the container test can
// assemble the PRODUCTION agent capability surface and assert every tool group is
// actually wired. A tool group whose port is left nil here is not advertised at
// all, and the only visible symptom in a running daemon is an agent that quietly
// has fewer tools than it should — which is why the wiring needs a test of its own
// rather than being trusted to review.
var NewAgentToolDepsForTest = newAgentToolDeps

// NewWorktreeChildCreatorForTest exposes the worktreeChildCreator adapter
// Promote's WorktreeCreator port is wired with, so a test can drive it over a
// REAL worktree.Usecase (real git, real resolveInherited, real branch
// generator) rather than the fake usecases/chat's own fixture stubs it with.
// That is the only way to prove the forced OwnWorktree=true actually reaches
// CreateChild end to end.
func NewWorktreeChildCreatorForTest(w worktree.Usecase) agentusecase.WorktreeCreator {
	return worktreeChildCreator{worktree: w}
}
