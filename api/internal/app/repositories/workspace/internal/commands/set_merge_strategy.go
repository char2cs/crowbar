package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// SetMergeStrategy writes only the mergeStrategy field (09 §4, 00 §5.3).
type SetMergeStrategy struct {
	ID       string
	Strategy gitdomain.MergeStrategy
}

func (c SetMergeStrategy) AggregateID() string {
	return c.ID
}

func (c SetMergeStrategy) EventName() string {
	return "workspace.merge_strategy_set." + c.ID
}

func (c SetMergeStrategy) ShouldSnapshot() bool {
	return false
}

func (c SetMergeStrategy) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("set merge strategy: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetMergeStrategy) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.MergeStrategy = c.Strategy
	return ws
}
