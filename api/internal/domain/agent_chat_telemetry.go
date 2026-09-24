package domain

import "time"

// AgentChatTelemetry is one chat's LAST reported provider usage report,
// persisted so it survives a daemon restart. ReportJSON is the engine's own
// Telemetry struct (context/rate-limit/cost/model) — opaque here, this
// package has no reason to know its shape, only to keep it. ObservedAt is
// pulled out of that blob into its own column so a caller can reason about
// staleness without decoding it.
type AgentChatTelemetry struct {
	ChatID     string    `gorm:"primaryKey" json:"chatId"`
	ReportJSON string    `json:"reportJson"`
	ObservedAt time.Time `json:"observedAt"`
}

// TableName pins the sqlite table the generic store auto-migrates and reads,
// independent of struct-name pluralisation.
func (AgentChatTelemetry) TableName() string { return "agent_chat_telemetry" }
