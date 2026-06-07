package dto

import (
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReviewMessageDTO is the wire shape of one append-only message inside a review
// thread (09 §3): its id, optional author, agent flag, body, and creation time
// rendered in RFC 3339.
type ReviewMessageDTO struct {
	ID        string `json:"id"`
	Author    string `json:"author,omitempty"`
	IsAgent   bool   `json:"isAgent"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

// ReviewThreadDTO is the wire shape of a branch-review comment thread (09 §3):
// its anchor (file, line, side), resolution status, ordered messages, and
// creation time. The messages slice is always non-nil so it serialises as []
// rather than null when the thread carries no messages.
type ReviewThreadDTO struct {
	ID         string                    `json:"id"`
	WsID       string                    `json:"wsId"`
	FilePath   string                    `json:"filePath"`
	LineNumber int                       `json:"lineNumber"`
	Side       domain.ReviewSide         `json:"side"`
	Status     domain.ReviewThreadStatus `json:"status"`
	IsResolved bool                      `json:"isResolved"`
	Messages   []ReviewMessageDTO        `json:"messages"`
	CreatedAt  string                    `json:"createdAt"`
}

// BranchReviewDTO is the wire shape of the composite branch-review read model
// returned by GET /v0/workspaces/:wsId/review (09 §2): the description, the
// merge strategy, the range diff, the comment threads, and the read-only chat
// conversations. The diff is normalised so its nested slices are non-nil, and
// the threads and conversations slices are always non-nil.
type BranchReviewDTO struct {
	Description   string              `json:"description"`
	MergeStrategy string              `json:"mergeStrategy"`
	Diff          MultiFileDiffDTO    `json:"diff"`
	Threads       []ReviewThreadDTO   `json:"threads"`
	Conversations []domain.BranchChat `json:"conversations"`
}

// ReviewMessageDTOFrom converts a domain ReviewMessage into its wire DTO,
// rendering the creation timestamp in RFC 3339.
func ReviewMessageDTOFrom(
	msg domain.ReviewMessage,
) ReviewMessageDTO {
	return ReviewMessageDTO{
		ID:        msg.ID,
		Author:    msg.Author,
		IsAgent:   msg.IsAgent,
		Body:      msg.Body,
		CreatedAt: msg.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ReviewThreadDTOFrom converts a domain ReviewThread into its wire DTO,
// guaranteeing a non-nil messages slice and surfacing the derived isResolved
// flag alongside the raw status.
func ReviewThreadDTOFrom(
	thread domain.ReviewThread,
) ReviewThreadDTO {
	messages := make([]ReviewMessageDTO, 0, len(thread.Messages))
	for _, msg := range thread.Messages {
		messages = append(messages, ReviewMessageDTOFrom(msg))
	}
	return ReviewThreadDTO{
		ID:         thread.ID,
		WsID:       thread.WsID,
		FilePath:   thread.FilePath,
		LineNumber: thread.LineNumber,
		Side:       thread.Side,
		Status:     thread.Status,
		IsResolved: thread.IsResolved(),
		Messages:   messages,
		CreatedAt:  thread.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ReviewThreadDTOList converts a slice of domain ReviewThreads into wire DTOs,
// returning a non-nil empty slice for empty input so the threads field
// serialises as [] rather than null.
func ReviewThreadDTOList(
	threads []domain.ReviewThread,
) []ReviewThreadDTO {
	out := make([]ReviewThreadDTO, 0, len(threads))
	for _, thread := range threads {
		out = append(out, ReviewThreadDTOFrom(thread))
	}
	return out
}

// BranchReviewDTOFrom converts the composite domain BranchReview into its wire
// DTO, normalising the diff's nested slices and guaranteeing non-nil threads and
// conversations slices.
func BranchReviewDTOFrom(
	review domain.BranchReview,
) BranchReviewDTO {
	conversations := review.Conversations
	if conversations == nil {
		conversations = []domain.BranchChat{}
	}
	return BranchReviewDTO{
		Description:   review.Description,
		MergeStrategy: string(review.MergeStrategy),
		Diff:          MultiFileDiffDTOFrom(review.Diff),
		Threads:       ReviewThreadDTOList(review.Threads),
		Conversations: conversations,
	}
}
