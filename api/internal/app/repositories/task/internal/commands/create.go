package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type CreateTask struct {
	ID           string
	RepositoryID string
	Title        string
	FlowPath     string
	InitialState string
	BranchName   string
	BaseBranch   string
	WorktreePath string
}

func (c CreateTask) AggregateID() string  { return c.ID }
func (c CreateTask) EventName() string    { return "task.created" }
func (c CreateTask) ShouldSnapshot() bool { return false }

func (c CreateTask) Validate(
	current *domain.Task,
) error {
	if current != nil {
		return asynxModels.ErrValidation
	}
	if c.RepositoryID == "" || c.Title == "" || c.InitialState == "" || c.BranchName == "" || c.BaseBranch == "" || c.WorktreePath == "" {
		return asynxModels.ErrValidation
	}
	return nil
}

func (c CreateTask) EmitEvent(
	_ *domain.Task,
) domain.Task {
	now := time.Now()
	return domain.Task{
		ID:           c.ID,
		RepositoryID: c.RepositoryID,
		Title:        c.Title,
		FlowPath:     c.FlowPath,
		Status:       domain.TaskStatusPending,
		CurrentState: c.InitialState,
		BranchName:   c.BranchName,
		BaseBranch:   c.BaseBranch,
		WorktreePath: c.WorktreePath,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
