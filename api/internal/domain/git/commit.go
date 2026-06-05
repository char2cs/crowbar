package git

import "time"

// Commit is a single entry in the git log (04 §3).
type Commit struct {
	Hash        string    `json:"hash"`
	ShortHash   string    `json:"shortHash"`
	Message     string    `json:"message"`
	Description string    `json:"description,omitempty"`
	Author      string    `json:"author"`
	Email       string    `json:"email,omitempty"`
	Date        time.Time `json:"date"`
}
