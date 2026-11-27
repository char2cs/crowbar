package domain

import "time"

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

func (TerminalSession) TableName() string {
	return "terminal_sessions"
}
