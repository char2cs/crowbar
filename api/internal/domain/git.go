package domain

import "time"

// GitCommit is a single parsed git log entry.
type GitCommit struct {
	SHA     string
	Message string
	Author  string
	Date    time.Time
}
