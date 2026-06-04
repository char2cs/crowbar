package domain

import "time"

// AgentRun is the agent execution aggregate (00 §5.5). Mutation commands beyond
// crash recovery are bridge-owned (post-spike).
type AgentRun struct {
	ID        string         `json:"id"`
	WsID      string         `json:"wsId"`
	ChatID    string         `json:"chatId"`
	Status    AgentRunStatus `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
}
