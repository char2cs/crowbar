package dto

import (
	"time"
)

// TerminalSessionDTO is the wire shape of a PTY session's lifecycle (00 §5.6):
// the owning chat, the launch profile, the active|detached|suspended|ended
// status, and the creation/termination timestamps. It is the
// Broadcaster[TerminalSessionDTO] payload; the raw PTY byte stream is a
// separate, non-broadcast WebSocket. ExitCode is only present on "ended"
// frames where the exit code is known (>=0).
//
// ChatID is the ONLY id here, and it is also the broadcast topic key. The
// former projectId/repoId/workspaceId triple is gone: under the flat
// /v0/chats/:chatId/terminals route none of the three appears in the URL, and
// a workspace id on the wire is exactly what spec §6 rejected — a resource a
// consumer could name independently of any chat.
type TerminalSessionDTO struct {
	ID        string     `json:"id"`
	ChatID    string     `json:"chatId"`
	ProfileID string     `json:"profileId,omitempty"`
	Status    string     `json:"status"`
	ExitCode  *int       `json:"exitCode,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

// TerminalSessionDTOFrom builds a lifecycle DTO from a session id and the chat
// that owns it. Terminal sessions are ephemeral (D6: no terminal_sessions
// view.db), so the DTO is assembled from the in-memory engine registry plus the
// resolving path context rather than a domain row. EndedAt stays nil; the
// "ended" frame is stamped by the caller when a session terminates.
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
