package storage

import "time"

func rowKey(chatID, id string) string {
	return chatID + "\x00" + id
}

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

type ChoiceRow struct {
	Key    string `gorm:"primaryKey;column:key"`
	ID     string `gorm:"column:id"`
	TurnID string `gorm:"column:turn_id;index"`
	ChatID string `gorm:"column:chat_id;index:idx_choice_chat_seq,priority:1"`
	Seq    int64  `gorm:"column:seq;index:idx_choice_chat_seq,priority:2"`

	Kind     string `gorm:"column:kind;index"`
	PromptID string `gorm:"column:prompt_id"`

	ToolID   string `gorm:"column:tool_id;index"`
	ToolName string `gorm:"column:tool_name;index"`

	Title    string `gorm:"column:title"`
	Question string `gorm:"column:question"`
	Mode     string `gorm:"column:mode"`
	Multi    bool   `gorm:"column:multi"`
	Options  string `gorm:"column:options"`

	Questions string `gorm:"column:questions"`
	Schema    string `gorm:"column:schema"`

	At         time.Time  `gorm:"column:at"`
	ResolvedAt *time.Time `gorm:"column:resolved_at;index"`
	Resolution string     `gorm:"column:resolution"`
}

func (ChoiceRow) TableName() string { return "agent_choices" }
