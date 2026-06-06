package domain

import "time"

// ReviewMessage is one append-only message inside a ReviewThread (09 §3).
type ReviewMessage struct {
	ID        string    `json:"id"`
	Author    string    `json:"author,omitempty"`
	IsAgent   bool      `json:"isAgent"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}
