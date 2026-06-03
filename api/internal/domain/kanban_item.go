package domain

import "time"

type KanbanItem struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // open string type — validated against flow.ItemStatuses at runtime
	AgentRunID  string    `json:"agent_run_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
