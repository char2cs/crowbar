package domain

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type ReviewThread struct {
	ID         string             `json:"id"`
	WsID       string             `json:"wsId"`
	FilePath   string             `json:"filePath"`
	LineNumber int                `json:"lineNumber"`
	StartLine  int                `json:"startLine"`
	EndLine    int                `json:"endLine"`
	Side       ReviewSide         `json:"side"`
	Status     ReviewThreadStatus `json:"status"`
	Messages   []ReviewMessage    `json:"messages"`
	CreatedAt  time.Time          `json:"createdAt"`
}

func (t ReviewThread) IsResolved() bool {
	return t.Status == ReviewThreadStatusResolved
}

func (t ReviewThread) NormalizedMessages() ReviewThread {
	if t.Messages == nil {
		t.Messages = []ReviewMessage{}
	}
	return t
}

func ReviewThreadNotFound(cause error) error {
	return &problem.Error{Code: problem.NotFound, Message: "review thread not found", Err: cause}
}
