package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TerminalProfileDTO is the wire shape of a server-stored PTY launch profile
// (00 §5.6): its id and display name plus the optional shell, startup directory,
// startup commands, icon, and color. StartupCommands is always a non-nil slice
// so the envelope carries [] rather than null when the profile defines none.
type TerminalProfileDTO struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Shell            string   `json:"shell,omitempty"`
	StartupDirectory string   `json:"startupDirectory,omitempty"`
	StartupCommands  []string `json:"startupCommands"`
	Icon             string   `json:"icon,omitempty"`
	Color            string   `json:"color,omitempty"`
}

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

// TerminalProfileDTOFrom converts a domain TerminalProfile into its wire DTO,
// normalising the startup commands to a non-nil slice.
func TerminalProfileDTOFrom(
	profile domain.TerminalProfile,
) TerminalProfileDTO {
	cmds := make([]string, 0, len(profile.StartupCommands))
	cmds = append(cmds, profile.StartupCommands...)
	return TerminalProfileDTO{
		ID:               profile.ID,
		Name:             profile.Name,
		Shell:            profile.Shell,
		StartupDirectory: profile.StartupDirectory,
		StartupCommands:  cmds,
		Icon:             profile.Icon,
		Color:            profile.Color,
	}
}

// TerminalProfileDTOList converts a slice of domain profiles into their wire
// DTOs, returning a non-nil slice so the envelope carries [] rather than null
// when the store is empty.
func TerminalProfileDTOList(
	profiles []domain.TerminalProfile,
) []TerminalProfileDTO {
	out := make([]TerminalProfileDTO, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, TerminalProfileDTOFrom(profile))
	}
	return out
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

// TerminalSessionDTOList converts the in-memory session ids owned by one chat
// into lifecycle DTOs sharing that chat, returning a non-nil slice so the
// envelope carries [] rather than null when the chat has no live sessions.
func TerminalSessionDTOList(
	sessionIDs []string,
	chatID string,
	profileID string,
	status string,
	createdAt time.Time,
) []TerminalSessionDTO {
	out := make([]TerminalSessionDTO, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		out = append(
			out,
			TerminalSessionDTOFrom(id, chatID, profileID, status, createdAt),
		)
	}
	return out
}
