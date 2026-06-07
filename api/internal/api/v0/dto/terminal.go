package dto

import (
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

// TerminalSessionDTO is the wire shape of a created PTY session: the id the
// frontend reads to open the terminal WebSocket.
type TerminalSessionDTO struct {
	SessionID string `json:"sessionId"`
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
