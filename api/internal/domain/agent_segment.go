package domain

import "time"

// AgentSegment is one provider stint within an AgentChat, embedded in the
// aggregate (no longer its own table). Invariant (enforced in command Validate):
// at most one segment with Status=="active" per AgentChat.
type AgentSegment struct {
	ID                string     `json:"id"`
	ChatID            string     `json:"chatId"` // kept until Task 10 rewrites the usecase; redundant in the embedded aggregate but the pre-rewrite usecase still references it
	ProviderID        string     `json:"providerId"`
	ProviderSessionID string     `json:"providerSessionId,omitempty"`
	CrowbarSegmentID  string     `json:"crowbarSegmentId"`
	TerminalSessionID string     `json:"terminalSessionId"`
	StartedAt         time.Time  `json:"startedAt"`
	EndedAt           *time.Time `json:"endedAt,omitempty"`
	Status            string     `json:"status"` // active | ended
}
