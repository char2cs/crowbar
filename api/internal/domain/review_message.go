package domain

import "time"

// ReviewMessage is one append-only message inside a ReviewThread. ProviderID and ChatID
// attribute an agent's message to its CLI and chat; they are opaque, empty for humans
// and for messages that predate them (never backfilled), so readers must degrade.
type ReviewMessage struct {
	ID         string    `json:"id"`
	Author     string    `json:"author,omitempty"`
	IsAgent    bool      `json:"isAgent"`
	ProviderID string    `json:"providerId,omitempty"`
	ChatID     string    `json:"chatId,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"createdAt"`
}
