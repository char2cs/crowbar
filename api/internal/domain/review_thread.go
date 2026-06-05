package domain

import "time"

// ReviewThread is the branch-review comment-thread aggregate (00 §6.3, 09 §3).
// Messages are kept inside the aggregate because a thread is bounded.
type ReviewThread struct {
	ID         string             `json:"id"`
	WsID       string             `json:"wsId"`
	FilePath   string             `json:"filePath"`
	LineNumber int                `json:"lineNumber"`
	Side       ReviewSide         `json:"side"`
	Status     ReviewThreadStatus `json:"status"`
	Messages   []ReviewMessage    `json:"messages"`
	CreatedAt  time.Time          `json:"createdAt"`
}

// IsResolved reports whether the thread is resolved (09 §3 read model).
func (t ReviewThread) IsResolved() bool {
	return t.Status == ReviewThreadStatusResolved
}
