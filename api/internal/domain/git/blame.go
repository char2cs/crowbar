package git

import (
	"errors"
	"time"
)

var ErrNoHistory = errors.New("git: path has no committed history")

type BlameEntry struct {
	LineNumber    int       `json:"lineNumber"`
	CommitHash    string    `json:"commitHash"`
	Author        string    `json:"author"`
	Email         string    `json:"email"`
	Date          time.Time `json:"date"`
	CommitMessage string    `json:"commitMessage"`
}
