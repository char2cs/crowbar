package dto

import (
	"time"
)

type TerminalSessionDTO struct {
	ID        string     `json:"id"`
	ChatID    string     `json:"chatId"`
	ProfileID string     `json:"profileId,omitempty"`
	Status    string     `json:"status"`
	ExitCode  *int       `json:"exitCode,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

func TerminalSessionDTOFrom(
	sessionID string,
	chatID string,
	profileID string,
	status string,
	createdAt time.Time,
) TerminalSessionDTO {
	return TerminalSessionDTO{
		ID:        sessionID,
		ChatID:    chatID,
		ProfileID: profileID,
		Status:    status,
		CreatedAt: createdAt,
	}
}
