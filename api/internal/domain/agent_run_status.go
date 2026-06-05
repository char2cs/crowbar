package domain

// AgentRunStatus is the AgentRun lifecycle (00 §6.2).
type AgentRunStatus string

const (
	AgentRunStatusPending     AgentRunStatus = "pending"
	AgentRunStatusRunning     AgentRunStatus = "running"
	AgentRunStatusDone        AgentRunStatus = "done"
	AgentRunStatusError       AgentRunStatus = "error"
	AgentRunStatusInterrupted AgentRunStatus = "interrupted"
)
