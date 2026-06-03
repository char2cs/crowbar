package domain

import "time"

type ConversationMessageRole string
type ConversationMessageType string

const (
	ConversationMessageRoleUser  ConversationMessageRole = "user"
	ConversationMessageRoleAgent ConversationMessageRole = "agent"

	ConversationMessageTypeText       ConversationMessageType = "text"
	ConversationMessageTypeToolCall   ConversationMessageType = "tool_call"
	ConversationMessageTypeToolResult ConversationMessageType = "tool_result"
)

type ConversationMessage struct {
	ID        string                  `gorm:"primaryKey" json:"id"`
	TaskID    string                  `gorm:"index" json:"task_id"`
	Role      ConversationMessageRole `json:"role"`
	Type      ConversationMessageType `json:"type"`
	Content   string                  `json:"content"`
	CreatedAt time.Time               `json:"created_at"`
}
