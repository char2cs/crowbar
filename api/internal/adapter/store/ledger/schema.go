package ledger

// schema is the chat ledger's tables. at is unix microseconds from the store
// clock; a heads row's open is 1 while its chat has unfinished work. A sessions
// row names the one chat a provider session is bound to. A dead letter is a hook
// the ingress could not place, kept with its payload so it can be read later.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS chat_ledger (
		chat_id TEXT NOT NULL, seq INTEGER NOT NULL, kind TEXT NOT NULL,
		turn_id TEXT NOT NULL DEFAULT '', scope TEXT NOT NULL DEFAULT '',
		at INTEGER NOT NULL,
		body BLOB NOT NULL,
		PRIMARY KEY (chat_id, seq)) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS chat_ledger_heads (
		chat_id TEXT PRIMARY KEY, last_seq INTEGER NOT NULL, open INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS chat_ledger_sessions (
		provider TEXT NOT NULL, session_id TEXT NOT NULL, chat_id TEXT NOT NULL,
		PRIMARY KEY (provider, session_id)) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS ledger_dead_letters (
		id INTEGER PRIMARY KEY AUTOINCREMENT, at INTEGER NOT NULL, chat_id TEXT NOT NULL DEFAULT '',
		runner_id TEXT NOT NULL, provider TEXT NOT NULL, event TEXT NOT NULL, reason TEXT NOT NULL,
		payload BLOB NOT NULL, truncated INTEGER NOT NULL)`,
}
