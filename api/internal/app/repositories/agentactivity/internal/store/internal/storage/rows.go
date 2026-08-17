// Package storage is the AgentActivity read model's durable shape.
//
// It holds ROWS, not one blob per chat. That is the whole reason this read model
// exists separately from the aggregate: the questions asked of it — which tools
// ran, which files were touched, what happened in the last hour, what is another
// agent in this workspace doing — are queries, and a JSON column cannot answer a
// query.
package storage

import "time"

// rowKey namespaces a provider-supplied id by its chat.
//
// Providers guarantee their ids are unique within a session, not across every
// chat this daemon has ever hosted. Two chats can therefore legitimately produce
// the same tool id, and a bare primary key would let the later one overwrite the
// earlier.
func rowKey(chatID, id string) string {
	return chatID + "\x00" + id
}

// TurnRow is one side of the conversation.
type TurnRow struct {
	Key        string     `gorm:"primaryKey;column:key"`
	ID         string     `gorm:"column:id"`
	ChatID     string     `gorm:"column:chat_id;index:idx_turn_chat_seq,priority:1"`
	Seq        int64      `gorm:"column:seq;index:idx_turn_chat_seq,priority:2"`
	Role       string     `gorm:"column:role"`
	ProviderID string     `gorm:"column:provider_id;index"`
	RunnerID   string     `gorm:"column:runner_id"`
	SessionID  string     `gorm:"column:session_id;index"`
	Text       string     `gorm:"column:text"`
	Effort     string     `gorm:"column:effort"`
	StartedAt  time.Time  `gorm:"column:started_at;index"`
	EndedAt    *time.Time `gorm:"column:ended_at"`
}

func (TurnRow) TableName() string { return "agent_turns" }

// ToolCallRow is one tool invocation. The payloads live in the content store;
// only their refs are here, so this table stays small enough to index.
type ToolCallRow struct {
	Key        string     `gorm:"primaryKey;column:key"`
	ID         string     `gorm:"column:id"`
	TurnID     string     `gorm:"column:turn_id;index"`
	ChatID     string     `gorm:"column:chat_id;index:idx_tool_chat_seq,priority:1"`
	Seq        int64      `gorm:"column:seq;index:idx_tool_chat_seq,priority:2"`
	Name       string     `gorm:"column:name;index"`
	Target     string     `gorm:"column:target;index"`
	RequestRef string     `gorm:"column:request_ref"`
	ResultRef  string     `gorm:"column:result_ref"`
	Status     string     `gorm:"column:status;index"`
	DurationMS int        `gorm:"column:duration_ms"`
	StartedAt  time.Time  `gorm:"column:started_at;index"`
	EndedAt    *time.Time `gorm:"column:ended_at"`
}

func (ToolCallRow) TableName() string { return "agent_tool_calls" }

type SubagentRow struct {
	Key       string     `gorm:"primaryKey;column:key"`
	ID        string     `gorm:"column:id"`
	TurnID    string     `gorm:"column:turn_id;index"`
	ChatID    string     `gorm:"column:chat_id;index:idx_subagent_chat_seq,priority:1"`
	Seq       int64      `gorm:"column:seq;index:idx_subagent_chat_seq,priority:2"`
	AgentType string     `gorm:"column:agent_type"`
	StartedAt time.Time  `gorm:"column:started_at"`
	EndedAt   *time.Time `gorm:"column:ended_at"`
}

func (SubagentRow) TableName() string { return "agent_subagents" }

type InterruptionRow struct {
	Key        string     `gorm:"primaryKey;column:key"`
	ID         string     `gorm:"column:id"`
	TurnID     string     `gorm:"column:turn_id;index"`
	ChatID     string     `gorm:"column:chat_id;index:idx_interruption_chat_seq,priority:1"`
	Seq        int64      `gorm:"column:seq;index:idx_interruption_chat_seq,priority:2"`
	Kind       string     `gorm:"column:kind;index"`
	Detail     string     `gorm:"column:detail"`
	At         time.Time  `gorm:"column:at"`
	ResolvedAt *time.Time `gorm:"column:resolved_at"`
}

func (InterruptionRow) TableName() string { return "agent_interruptions" }
