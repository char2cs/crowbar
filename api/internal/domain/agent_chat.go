package domain

import "time"

// AgentChat is a Crowbar-owned agentic conversation tracked across provider
// segments (00 agentic-engine spec §6). Distinct from the event-sourced Chat.
type AgentChat struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	WorkspaceID     string    `gorm:"index"      json:"workspaceId"`
	Title           string    `json:"title"`
	ActiveSegmentID string    `json:"activeSegmentId"`
	CreatedAt       time.Time `json:"createdAt"`
}

func (AgentChat) TableName() string { return "agent_chats" }
