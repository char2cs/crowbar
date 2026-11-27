package usecases

import (
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
)

func NewAgentToolDepsForTest(
	minter *agentusecase.TokenMinter,
	repos *repositories.Container,
	review agentusecase.ToolReviewReader,
	threadBroadcast agentusecase.ToolThreadBroadcast,
	workspaces agentusecase.ToolWorkspaceBranchRenamer,
) (agentusecase.ToolDeps, error) {
	return newAgentToolDeps(minter, repos, review, threadBroadcast, workspaces, nil)
}

func NewWorktreeChildCreatorForTest(w workspace.Usecase) agentusecase.WorktreeCreator {
	return worktreeChildCreator{worktree: w}
}
