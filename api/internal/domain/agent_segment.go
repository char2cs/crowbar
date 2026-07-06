package domain

import "time"

// AgentSegment is one provider stint within an AgentChat.
type AgentSegment struct {
	ID                string     `gorm:"primaryKey" json:"id"`
	ChatID            string     `gorm:"index"      json:"chatId"`
	ProviderID        string     `json:"providerId"`
	ProviderSessionID string     `gorm:"index"      json:"providerSessionId"`
	CrowbarSegmentID  string     `gorm:"index"      json:"crowbarSegmentId"`
	TerminalSessionID string     `json:"terminalSessionId"`
	TranscriptPath    string     `json:"transcriptPath"`
	StartedAt         time.Time  `json:"startedAt"`
	EndedAt           *time.Time `json:"endedAt,omitempty"`
	Status            string     `json:"status"`
}

func (AgentSegment) TableName() string { return "agent_segments" }
