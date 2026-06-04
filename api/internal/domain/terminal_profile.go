package domain

// TerminalProfile is a server-stored PTY launch profile (00 §5.6).
type TerminalProfile struct {
	ID               string   `gorm:"primaryKey" json:"id"`
	Name             string   `json:"name"`
	Shell            string   `json:"shell,omitempty"`
	StartupDirectory string   `json:"startupDirectory,omitempty"`
	StartupCommands  []string `gorm:"serializer:json" json:"startupCommands"`
	Icon             string   `json:"icon,omitempty"`
	Color            string   `json:"color,omitempty"`
}

func (TerminalProfile) TableName() string {
	return "terminal_profiles"
}
