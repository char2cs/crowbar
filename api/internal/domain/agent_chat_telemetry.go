package domain

import "time"

// AgentChatTelemetry is one chat's last reported provider usage, persisted across
// restarts. ReportJSON is the engine's Telemetry, opaque here; ObservedAt is its own
// column so callers can judge staleness without decoding the blob.
type AgentChatTelemetry struct {
	ChatID     string    `gorm:"primaryKey" json:"chatId"`
	ReportJSON string    `json:"reportJson"`
	ObservedAt time.Time `json:"observedAt"`
}

// TableName pins the sqlite table the generic store auto-migrates and reads,
// independent of struct-name pluralisation.
func (AgentChatTelemetry) TableName() string { return "agent_chat_telemetry" }
