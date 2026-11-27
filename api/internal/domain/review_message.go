package domain

import "time"

type ReviewMessage struct {
	ID         string    `json:"id"`
	Author     string    `json:"author,omitempty"`
	IsAgent    bool      `json:"isAgent"`
	ProviderID string    `json:"providerId,omitempty"`
	ChatID     string    `json:"chatId,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"createdAt"`
}
