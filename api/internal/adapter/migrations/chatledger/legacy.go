// Package chatledger reads what the pre-ledger daemon recorded about a chat,
// for the upgrade steps that restate it. Only agent_interruptions is still
// written, by the activity store this daemon keeps open.
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
)

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
