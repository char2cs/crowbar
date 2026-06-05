package domain

import "time"

// MultiFileDiff is the diff for a commit (04 §3).
type MultiFileDiff struct {
	CommitHash        string     `json:"commitHash,omitempty"`
	CommitMessage     string     `json:"commitMessage,omitempty"`
	CommitDescription string     `json:"commitDescription,omitempty"`
	CommitAuthor      string     `json:"commitAuthor,omitempty"`
	CommitDate        *time.Time `json:"commitDate,omitempty"`
	Files             []FileDiff `json:"files"`
	TotalFiles        int        `json:"totalFiles"`
	TotalAdditions    int        `json:"totalAdditions"`
	TotalDeletions    int        `json:"totalDeletions"`
}
