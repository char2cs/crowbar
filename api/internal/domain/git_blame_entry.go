package domain

import "time"

// BlameEntry annotates a single line with the commit that last changed it (04 §3).
type BlameEntry struct {
	LineNumber    int       `json:"lineNumber"`
	CommitHash    string    `json:"commitHash"`
	Author        string    `json:"author"`
	Email         string    `json:"email"`
	Date          time.Time `json:"date"`
	CommitMessage string    `json:"commitMessage"`
}
