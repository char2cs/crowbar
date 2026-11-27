package domain

import "time"

type AgentChatTelemetry struct {
	ChatID     string    `gorm:"primaryKey" json:"chatId"`
	ReportJSON string    `json:"reportJson"`
	ObservedAt time.Time `json:"observedAt"`
}

func (AgentChatTelemetry) TableName() string { return "agent_chat_telemetry" }
