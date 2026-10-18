package ledger

// schema is the chat ledger's tables. at is unix microseconds from the store
// clock; a heads row's open is 1 while its chat has unfinished work.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS chat_ledger (
		chat_id TEXT NOT NULL, seq INTEGER NOT NULL, kind TEXT NOT NULL,
		turn_id TEXT NOT NULL DEFAULT '', scope TEXT NOT NULL DEFAULT '',
		at INTEGER NOT NULL,
		body BLOB NOT NULL,
		PRIMARY KEY (chat_id, seq)) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS chat_ledger_heads (
		chat_id TEXT PRIMARY KEY, last_seq INTEGER NOT NULL, open INTEGER NOT NULL)`,
}
