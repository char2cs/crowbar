package domain

import "time"

// TerminalSession is the durable metadata record for a terminal PTY session.
// It lives as a row in the global state/view.db so the daemon can restore
// sessions after idle-suspend or daemon restart (Phase 2/3 of terminal
// session persistence). This supersedes decision D6 ("terminals are
// ephemeral; no terminal_sessions view.db").
type TerminalSession struct {
	SessionID    string    `gorm:"primaryKey" json:"sessionId"`
	WorkspaceID  string    `gorm:"index"      json:"workspaceId"`
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
