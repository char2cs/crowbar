package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// OpenReviewThread creates a ReviewThread anchored to a diff line, with its first
// message (09 §3).
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
			ID:        c.MessageID,
			Author:    c.Author,
			IsAgent:   c.IsAgent,
			Body:      c.Body,
			CreatedAt: c.Now,
		}},
		CreatedAt: c.Now,
	}
}
