package dto

import (
	"github.com/char2cs/crowbar/api/internal/domain"
)

type ReviewMessageDTO struct {
	ID         string `json:"id"`
	Author     string `json:"author,omitempty"`
	IsAgent    bool   `json:"isAgent"`
	ProviderID string `json:"providerId,omitempty"`
	ChatID     string `json:"chatId,omitempty"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
}

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

type BranchReviewDTO struct {
	Description   string            `json:"description"`
	MergeStrategy string            `json:"mergeStrategy"`
	Threads       []ReviewThreadDTO `json:"threads"`
}

func ReviewMessageDTOFrom(
	msg domain.ReviewMessage,
) ReviewMessageDTO {
	return ReviewMessageDTO{
		ID:         msg.ID,
		Author:     msg.Author,
		IsAgent:    msg.IsAgent,
		ProviderID: msg.ProviderID,
		ChatID:     msg.ChatID,
		Body:       msg.Body,
		CreatedAt:  msg.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

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

func ReviewThreadDTOList(
	threads []domain.ReviewThread,
) []ReviewThreadDTO {
	out := make([]ReviewThreadDTO, 0, len(threads))
	for _, thread := range threads {
		out = append(out, ReviewThreadDTOFrom(thread))
	}
	return out
}

func BranchReviewDTOFrom(
	review domain.BranchReview,
) BranchReviewDTO {
	return BranchReviewDTO{
		Description:   review.Description,
		MergeStrategy: string(review.MergeStrategy),
		Threads:       ReviewThreadDTOList(review.Threads),
	}
}
