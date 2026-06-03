package domain

import "time"

type AgentRunStatus string

const (
	AgentRunStatusRunning     AgentRunStatus = "running"
	AgentRunStatusCompleted   AgentRunStatus = "completed"
	AgentRunStatusFailed      AgentRunStatus = "failed"
	AgentRunStatusInterrupted AgentRunStatus = "interrupted"
)

type AgentRunOutput struct {
	StateName string `json:"state_name"`
	Output    string `json:"output"`
}

type AgentRun struct {
	ID        string           `json:"id"`
	TaskID    string           `json:"task_id"`
	StateName string           `json:"state_name"`
	Status    AgentRunStatus   `json:"status"`
	Token     string           `gorm:"-" json:"token,omitempty"`
	Outputs   []AgentRunOutput `json:"outputs"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}
