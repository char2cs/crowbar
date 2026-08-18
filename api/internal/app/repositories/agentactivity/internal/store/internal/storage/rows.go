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
	Error      string     `gorm:"column:error"`
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

// ChoiceRow is one prompt the agent put to a human.
//
// Options is a JSON array rather than a child table on purpose: nothing queries
// an option — they are read only as part of the prompt that offered them — and a
// join per pending prompt would buy nothing. The prompt itself IS queried, by
// chat and by whether it is still pending, and those are columns.
type ChoiceRow struct {
	Key    string `gorm:"primaryKey;column:key"`
	ID     string `gorm:"column:id"`
	TurnID string `gorm:"column:turn_id;index"`
	ChatID string `gorm:"column:chat_id;index:idx_choice_chat_seq,priority:1"`
	Seq    int64  `gorm:"column:seq;index:idx_choice_chat_seq,priority:2"`

	Kind     string `gorm:"column:kind;index"`
	PromptID string `gorm:"column:prompt_id"`
	// ToolID and ToolName are how a completion finds the prompt it answered. Both
	// are indexed because the resolution sweep runs on every tool completion, which
	// is the highest-frequency event in the system.
	ToolID   string `gorm:"column:tool_id;index"`
	ToolName string `gorm:"column:tool_name;index"`

	Title    string `gorm:"column:title"`
	Question string `gorm:"column:question"`
	Mode     string `gorm:"column:mode"`
	Multi    bool   `gorm:"column:multi"`
	Options  string `gorm:"column:options"`
	Schema   string `gorm:"column:schema"`

	At         time.Time  `gorm:"column:at"`
	ResolvedAt *time.Time `gorm:"column:resolved_at;index"`
	Resolution string     `gorm:"column:resolution"`
}

func (ChoiceRow) TableName() string { return "agent_choices" }
