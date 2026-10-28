package git

import (
	"errors"
	"time"
)

// ErrNoHistory signals a path with no committed history to blame: a new or
// untracked file, or a repository with no commits yet.
var ErrNoHistory = errors.New("git: path has no committed history")

// BlameEntry annotates a single line with the commit that last changed it (04 §3).
type BlameEntry struct {
	LineNumber    int       `json:"lineNumber"`
	CommitHash    string    `json:"commitHash"`
	Author        string    `json:"author"`
	Email         string    `json:"email"`
	Date          time.Time `json:"date"`
	CommitMessage string    `json:"commitMessage"`
}
