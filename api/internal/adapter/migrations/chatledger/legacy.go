// Package chatledger reads what the pre-ledger daemon recorded about a chat
// and restates it as ledger rows at the upgrade boot. Nothing writes these
// tables any more.
package chatledger

import "time"

// The databases, under the home's store directory, the legacy tables live in.
const (
	RunnerDB   = "agent_runner.db"
	ActivityDB = "agent_activity.db"
)

// The legacy tables as the old daemon's ORM created them; a DATETIME column is
// what lets the driver read a timestamp back as a time.
const (
	ConversationsDDL = `CREATE TABLE agent_chat_conversations (
	chat_id TEXT, session_id TEXT, workspace_id TEXT, provider_id TEXT,
	first_seen_at DATETIME, last_active_at DATETIME,
	PRIMARY KEY (chat_id, session_id))`

	PlacementsDDL = `CREATE TABLE agent_chat_placements (
	chat_id TEXT, provider_id TEXT, workspace_id TEXT, last_placed_at DATETIME,
	PRIMARY KEY (chat_id, provider_id))`

	RunnersDDL = `CREATE TABLE agent_runners (
	id TEXT PRIMARY KEY, workspace_id TEXT, provider_id TEXT, terminal_session TEXT,
	current_chat_id TEXT, launch_model TEXT, launch_effort TEXT, reported_model TEXT,
	reported_effort TEXT, launch_permission_level TEXT, started_at DATETIME)`

	InterruptionsDDL = `CREATE TABLE agent_interruptions (
	key TEXT PRIMARY KEY, id TEXT, turn_id TEXT, chat_id TEXT, seq INTEGER,
	display_order INTEGER, kind TEXT, detail TEXT, at DATETIME, resolved_at DATETIME)`

	TurnsDDL = `CREATE TABLE agent_turns (
	key TEXT PRIMARY KEY, id TEXT, chat_id TEXT, seq INTEGER, display_order INTEGER,
	item_index INTEGER, role TEXT, provider_id TEXT, runner_id TEXT, session_id TEXT,
	text TEXT, effort TEXT, status TEXT, diff TEXT, plan TEXT, plan_updated_at DATETIME,
	started_at DATETIME, ended_at DATETIME)`

	ToolCallsDDL = `CREATE TABLE agent_tool_calls (
	key TEXT PRIMARY KEY, id TEXT, turn_id TEXT, chat_id TEXT, seq INTEGER, name TEXT,
	kind TEXT, locations TEXT, target TEXT, request_ref TEXT, result_ref TEXT, diff TEXT,
	status TEXT, error TEXT, duration_ms INTEGER, subagent_id TEXT,
	started_at DATETIME, ended_at DATETIME)`

	SubagentsDDL = `CREATE TABLE agent_subagents (
	key TEXT PRIMARY KEY, id TEXT, turn_id TEXT, chat_id TEXT, seq INTEGER,
	agent_type TEXT, started_at DATETIME, ended_at DATETIME, messages TEXT)`

	ChoicesDDL = `CREATE TABLE agent_choices (
	key TEXT PRIMARY KEY, id TEXT, turn_id TEXT, chat_id TEXT, seq INTEGER, kind TEXT,
	prompt_id TEXT, tool_id TEXT, tool_name TEXT, title TEXT, question TEXT, mode TEXT,
	multi NUMERIC, options TEXT, questions TEXT, schema TEXT, at DATETIME,
	resolved_at DATETIME, resolution TEXT, auto_approved NUMERIC, answered_option_ids TEXT)`
)

// PromptsDir is the directory, under each chat's directory in the home's
// chats directory, the old daemon journalled its prompt deliveries in.
const PromptsDir = "prompt-requests"

// providerSwitched is the interruption kind whose detail names the provider
// the chat was switched to.
const providerSwitched = "provider_switched"

// placed is one legacy row read as the provider it names on a chat, and when.
type placed struct {
	ChatID, ProviderID string
	At                 time.Time
}

// LegacyRunner is a CLI the pre-ledger daemon still held running on a chat:
// one it never saw exit.
type LegacyRunner struct {
	ID, ChatID, ProviderID string
}

// LegacyConversation is an agent_chat_conversations row: a provider session
// the chat ran on.
type LegacyConversation struct {
	ChatID, ProviderID, SessionID string
	FirstSeenAt, LastActiveAt     time.Time
}

// LegacyTurn is an agent_turns row: one message of the transcript.
type LegacyTurn struct {
	ID, ChatID, Role, ProviderID, RunnerID, SessionID, Text, Effort, Status, Diff, Plan string
	Seq, DisplayOrder                                                                   int64
	ItemIndex                                                                           int
	StartedAt                                                                           time.Time
	EndedAt                                                                             *time.Time
}

// LegacyTool is an agent_tool_calls row.
type LegacyTool struct {
	ID, TurnID, ChatID, Name, Kind, Locations, Target, RequestRef, ResultRef, Diff, Status, Error, SubagentID string
	Seq                                                                                                       int64
	DurationMS                                                                                                int
	StartedAt                                                                                                 time.Time
	EndedAt                                                                                                   *time.Time
}

// LegacySubagent is an agent_subagents row; Messages is its JSON-encoded replies.
type LegacySubagent struct {
	ID, TurnID, ChatID, AgentType, Messages string
	Seq                                     int64
	StartedAt                               time.Time
	EndedAt                                 *time.Time
}

// LegacyInterruption is an agent_interruptions row.
type LegacyInterruption struct {
	ID, TurnID, ChatID, Kind, Detail string
	Seq, DisplayOrder                int64
	At                               time.Time
	ResolvedAt                       *time.Time
}

// LegacyChoice is an agent_choices row; its JSON columns are kept as text.
type LegacyChoice struct {
	ID, TurnID, ChatID, Kind, PromptID, ToolID, ToolName, Title, Question, Mode string
	Options, Questions, Schema, Resolution, AnsweredOptionIDs                   string
	Multi                                                                       bool
	Seq                                                                         int64
	At                                                                          time.Time
	ResolvedAt                                                                  *time.Time
}

// LegacyPrompt is one prompt-requests/<id>.json record, in its on-disk names.
type LegacyPrompt struct {
	RequestID  string    `json:"requestId"`
	Text       string    `json:"text"`
	State      string    `json:"state"`
	ProviderID string    `json:"providerId"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// LegacyChat is everything the old daemon recorded about one chat. Unreadable
// names each record the reader could not decode, with why.
type LegacyChat struct {
	ChatID        string
	Conversations []LegacyConversation
	Turns         []LegacyTurn
	Tools         []LegacyTool
	Subagents     []LegacySubagent
	Interruptions []LegacyInterruption
	Choices       []LegacyChoice
	Prompts       []LegacyPrompt
	Runners       []LegacyRunner
	Unreadable    []string
}
