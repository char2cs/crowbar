package ledger

// schema is the chat ledger's tables. at is unix microseconds from the store clock;
// heads.open is 1 while the chat has unfinished work; a sessions row binds a provider
// session to one chat; a dead letter is an unplaceable hook kept for deadLetterRetention.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS chat_ledger (
		chat_id TEXT NOT NULL, seq INTEGER NOT NULL, kind TEXT NOT NULL,
		turn_id TEXT NOT NULL DEFAULT '', scope TEXT NOT NULL DEFAULT '',
		at INTEGER NOT NULL,
		body BLOB NOT NULL,
		PRIMARY KEY (chat_id, seq)) WITHOUT ROWID`,
	// A feed's window finds where each of its turns begins.
	`CREATE INDEX IF NOT EXISTS chat_ledger_by_turn ON chat_ledger (chat_id, turn_id, seq)`,
	`CREATE TABLE IF NOT EXISTS chat_ledger_heads (
		chat_id TEXT PRIMARY KEY, last_seq INTEGER NOT NULL, open INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS chat_ledger_sessions (
		provider TEXT NOT NULL, session_id TEXT NOT NULL, chat_id TEXT NOT NULL,
		PRIMARY KEY (provider, session_id)) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS ledger_dead_letters (
		id INTEGER PRIMARY KEY AUTOINCREMENT, at INTEGER NOT NULL, chat_id TEXT NOT NULL DEFAULT '',
		runner_id TEXT NOT NULL, provider TEXT NOT NULL, event TEXT NOT NULL, reason TEXT NOT NULL,
		payload BLOB NOT NULL, truncated INTEGER NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS ledger_dead_letters_by_reason ON ledger_dead_letters (reason)`,
	`CREATE INDEX IF NOT EXISTS ledger_dead_letters_by_at ON ledger_dead_letters (at)`,
}
