package domain

import "time"

// TerminalSession is the durable metadata record for a terminal PTY session.
// It lives as a row in the global state/view.db so the daemon can restore
// sessions after idle-suspend or daemon restart (Phase 2/3 of terminal
// session persistence). This supersedes decision D6 ("terminals are
// ephemeral; no terminal_sessions view.db").
//
// A session is owned by the CHAT that opened it, not by the worktree it runs
// in (spec 2026-09-02-chat-scoped-api-design §4.2's "owned by one chat"
// bucket). Sibling chats routinely share one worktree — batch import and
// repo-add both mint many chats over shared lineage — and a shell one of them
// opened is not a shell the others may see, write to, or kill. ChatID is what
// makes that true; the worktree still supplies the session's CWD, which is a
// different question with a different answer.
type TerminalSession struct {
	SessionID    string    `gorm:"primaryKey" json:"sessionId"`
	ChatID       string    `gorm:"index"      json:"chatId"`
	ProjectID    string    `json:"projectId"`
	RepoID       string    `json:"repoId"`
	CWD          string    `json:"cwd"`
	Shell        string    `json:"shell"`
	ProfileID    string    `json:"profileId"`
	State        string    `json:"state"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
}

// TableName returns the GORM table name for TerminalSession.
func (TerminalSession) TableName() string {
	return "terminal_sessions"
}
