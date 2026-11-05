package dto

import (
	"time"
)

// TerminalSessionDTO is the broadcast lifecycle of a PTY session (the byte stream is a
// separate WebSocket). ChatID is its only id and the topic key, so no workspace id is
// nameable apart from a chat. ExitCode is present only on "ended" frames with a known code.
type TerminalSessionDTO struct {
	ID        string     `json:"id"`
	ChatID    string     `json:"chatId"`
	ProfileID string     `json:"profileId,omitempty"`
	Status    string     `json:"status"`
	ExitCode  *int       `json:"exitCode,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

// TerminalSessionDTOFrom builds a lifecycle DTO from in-memory session facts, since
// terminal sessions are ephemeral and have no stored row. The caller stamps EndedAt.
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
