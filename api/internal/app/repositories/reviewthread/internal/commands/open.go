package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// OpenReviewThread creates a ReviewThread anchored to a diff line (or, with no anchor, a
// general comment), with its first message (09 §3).
//
// ProviderID and ChatID attribute the first message to the agent that wrote it;
// a human open leaves both empty. They are not validated: attribution is
// decoration on a message, never authority, so a write that carries none is a
// perfectly valid thread.
type OpenReviewThread struct {
	ID         string
	WsID       string
	FilePath   string
	LineNumber int
	StartLine  int
	EndLine    int
	Side       domain.ReviewSide
	MessageID  string
	Author     string
	IsAgent    bool
	ProviderID string
	ChatID     string
	Body       string
	Now        time.Time
}

func (c OpenReviewThread) AggregateID() string {
	return c.ID
}

func (c OpenReviewThread) EventName() string {
	return "review_thread.opened." + c.ID
}

func (c OpenReviewThread) ShouldSnapshot() bool {
	return false
}

func (c OpenReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current != nil {
		return fmt.Errorf("open review thread: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" || c.MessageID == "" {
		return fmt.Errorf("open review thread: missing ids: %w", asynxModels.ErrValidation)
	}
	return c.validateAnchor()
}

// validateAnchor accepts either no anchor at all (a general comment: empty path,
// every line 0) or a complete one; a half anchor would be a thread no diff line
// can ever show.
func (c OpenReviewThread) validateAnchor() error {
	if c.FilePath == "" && c.LineNumber == 0 && c.StartLine == 0 && c.EndLine == 0 {
		return nil
	}
	start, end := c.StartLine, c.EndLine
	if start == 0 {
		start = c.LineNumber
	}
	if end == 0 {
		end = c.LineNumber
	}
	if c.FilePath == "" || c.LineNumber < 0 || start < 1 || end < start {
		return fmt.Errorf("open review thread: incomplete anchor: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c OpenReviewThread) EmitEvent(
	_ *domain.ReviewThread,
) domain.ReviewThread {
	startLine := c.StartLine
	if startLine == 0 {
		startLine = c.LineNumber
	}
	endLine := c.EndLine
	if endLine == 0 {
		endLine = c.LineNumber
	}
	return domain.ReviewThread{
		ID:         c.ID,
		WsID:       c.WsID,
		FilePath:   c.FilePath,
		LineNumber: c.LineNumber,
		StartLine:  startLine,
		EndLine:    endLine,
		Side:       c.Side,
		Status:     domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{{
			ID:         c.MessageID,
			Author:     c.Author,
			IsAgent:    c.IsAgent,
			ProviderID: c.ProviderID,
			ChatID:     c.ChatID,
			Body:       c.Body,
			CreatedAt:  c.Now,
		}},
		CreatedAt: c.Now,
	}
}
