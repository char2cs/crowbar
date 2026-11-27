package models

import "time"

type Runner struct {
	ID              string `json:"id"`
	WorkspaceID     string `json:"workspaceId"`
	ProviderID      string `json:"providerId"`
	TerminalSession string `json:"terminalSessionId"`

	CurrentChatID         string `json:"currentChatId,omitempty"`
	LaunchModel           string `json:"launchModel,omitempty"`
	LaunchEffort          string `json:"launchEffort,omitempty"`
	ReportedModel         string `json:"reportedModel,omitempty"`
	ReportedEffort        string `json:"reportedEffort,omitempty"`
	LaunchPermissionLevel string `json:"launchPermissionLevel,omitempty"`

	StartedAt time.Time `json:"startedAt"`
}

type ChatConversation struct {
	ChatID       string    `json:"chatId"`
	ProviderID   string    `json:"providerId"`
	SessionID    string    `json:"sessionId"`
	FirstSeenAt  time.Time `json:"firstSeenAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
}
