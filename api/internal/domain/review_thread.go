package domain

import "time"

// ReviewThread is the branch-review comment-thread aggregate (00 §6.3, 09 §3).
// Messages are kept inside the aggregate because a thread is bounded.
type ReviewThread struct {
	ID         string `json:"id"`
	WsID       string `json:"wsId"`
	FilePath   string `json:"filePath"`
	LineNumber int    `json:"lineNumber"`
	// StartLine and EndLine bound a multi-line comment range on the same Side.
	// For a single-line comment StartLine == EndLine == LineNumber.
	StartLine int                `json:"startLine"`
	EndLine   int                `json:"endLine"`
	Side      ReviewSide         `json:"side"`
	Status    ReviewThreadStatus `json:"status"`
	Messages  []ReviewMessage    `json:"messages"`
	CreatedAt time.Time          `json:"createdAt"`
}

// IsResolved reports whether the thread is resolved (09 §3 read model).
func (t ReviewThread) IsResolved() bool {
	return t.Status == ReviewThreadStatusResolved
}

// NormalizedMessages returns the thread with a non-nil Messages slice so the
// wire contract serializes "messages": [] rather than null when a thread has no
// messages, matching how the rest of the API normalizes empty slices.
func (t ReviewThread) NormalizedMessages() ReviewThread {
	if t.Messages == nil {
		t.Messages = []ReviewMessage{}
	}
	return t
}
