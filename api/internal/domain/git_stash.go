package domain

import "time"

// Stash is a single stash entry (04 §3).
type Stash struct {
	ID           string    `json:"id"`
	Message      string    `json:"message"`
	Date         time.Time `json:"date"`
	FilesChanged int       `json:"filesChanged"`
}
