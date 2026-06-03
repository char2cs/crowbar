package usecases

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// AgentRunUsecase is the public contract for agent run operations.
type AgentRunUsecase interface {
	List(
		ctx context.Context,
		taskID string,
	) ([]domain.AgentRun, error)

	Interrupt(
		ctx context.Context,
		id string,
	) error
}

type agentRunUsecase struct {
	repo repositories.AgentRun
}

// NewAgentRun wires an AgentRun repository into an AgentRunUsecase.
func NewAgentRun(
	repo repositories.AgentRun,
) AgentRunUsecase {
	return &agentRunUsecase{repo: repo}
}

func (u *agentRunUsecase) List(
	ctx context.Context,
	taskID string,
) ([]domain.AgentRun, error) {
	return u.repo.ListByTask(ctx, taskID)
}

func (u *agentRunUsecase) Interrupt(
	ctx context.Context,
	id string,
) error {
	return u.repo.InterruptAgentRun(ctx, id)
}
