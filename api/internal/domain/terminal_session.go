package domain

import "time"

// TerminalSession is the durable metadata of a terminal PTY session in state/view.db,
// so sessions survive idle-suspend and restarts. It is owned by the chat that opened
// it, not its worktree: sibling chats sharing a worktree must not see each other's shells.
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
