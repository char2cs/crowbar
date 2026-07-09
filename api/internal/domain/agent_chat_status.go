package domain

// AgentChatStatus is the lifecycle state of an AgentChat aggregate.
type AgentChatStatus string

const (
	AgentChatStatusActive   AgentChatStatus = "active"
	AgentChatStatusArchived AgentChatStatus = "archived"
	AgentChatStatusDeleted  AgentChatStatus = "deleted"
)
