package domain

import "time"

type TaskStatus string

const (
	TaskStatusPending  TaskStatus = "pending"
	TaskStatusRunning  TaskStatus = "running"
	TaskStatusPaused   TaskStatus = "paused"
	TaskStatusComplete TaskStatus = "complete"
	TaskStatusArchived TaskStatus = "archived"
)

type Task struct {
	ID           string     `json:"id"`
	RepositoryID string     `json:"repository_id"`
	Title        string     `json:"title"`
	Status       TaskStatus `json:"status"`
	CurrentState string     `json:"current_state"` // flow state name
	FlowPath     string     `json:"flow_path"`     // "" = builtin
	BranchName   string     `json:"branch_name"`
	BaseBranch   string     `json:"base_branch"`
	WorktreePath string     `json:"worktree_path"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
