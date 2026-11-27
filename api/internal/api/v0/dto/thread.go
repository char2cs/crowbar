package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ThreadReplyDTO struct {
	ID         string    `json:"id"`
	ThreadID   string    `json:"threadId"`
	Body       string    `json:"body"`
	Author     string    `json:"author"`
	IsAgent    bool      `json:"isAgent"`
	ProviderID string    `json:"providerId,omitempty"`
	ChatID     string    `json:"chatId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ThreadDTO struct {
	ID          string           `json:"id"`
	ProjectID   string           `json:"projectId"`
	RepoID      string           `json:"repoId"`
	WorkspaceID string           `json:"workspaceId"`
	FilePath    string           `json:"filePath"`
	Line        int              `json:"line"`
	StartLine   int              `json:"startLine"`
	EndLine     int              `json:"endLine"`
	Side        string           `json:"side"`
	MessageID   string           `json:"messageId"`
	Body        string           `json:"body"`
	Author      string           `json:"author"`
	IsAgent     bool             `json:"isAgent"`
	ProviderID  string           `json:"providerId,omitempty"`
	ChatID      string           `json:"chatId,omitempty"`
	Resolved    bool             `json:"resolved"`
	CreatedAt   time.Time        `json:"createdAt"`
	Replies     []ThreadReplyDTO `json:"replies"`
	Deleted     bool             `json:"deleted,omitempty"`
}

func ThreadDTOFrom(
	rt domain.ReviewThread,
	projectID string,
	repoID string,
) ThreadDTO {
	root := rootMessage(rt)
	return ThreadDTO{
		ID:          rt.ID,
		ProjectID:   projectID,
		RepoID:      repoID,
		WorkspaceID: rt.WsID,
		FilePath:    rt.FilePath,
		Line:        rt.LineNumber,
		StartLine:   rt.StartLine,
		EndLine:     rt.EndLine,
		Side:        string(rt.Side),
		MessageID:   root.ID,
		Body:        root.Body,
		Author:      root.Author,
		IsAgent:     root.IsAgent,
		ProviderID:  root.ProviderID,
		ChatID:      root.ChatID,
		Resolved:    rt.IsResolved(),
		CreatedAt:   rt.CreatedAt,
		Replies:     threadReplies(rt),
	}
}

func rootMessage(
	rt domain.ReviewThread,
) domain.ReviewMessage {
	if len(rt.Messages) == 0 {
		return domain.ReviewMessage{}
	}
	return rt.Messages[0]
}

func threadReplies(
	rt domain.ReviewThread,
) []ThreadReplyDTO {
	out := make([]ThreadReplyDTO, 0, len(rt.Messages))
	for i, msg := range rt.Messages {
		if i == 0 {
			continue
		}
		out = append(out, ThreadReplyDTO{
			ID:         msg.ID,
			ThreadID:   rt.ID,
			Body:       msg.Body,
			Author:     msg.Author,
			IsAgent:    msg.IsAgent,
			ProviderID: msg.ProviderID,
			ChatID:     msg.ChatID,
			CreatedAt:  msg.CreatedAt,
		})
	}
	return out
}

func ThreadDTOList(
	threads []domain.ReviewThread,
	projectID string,
	repoID string,
) []ThreadDTO {
	out := make([]ThreadDTO, 0, len(threads))
	for _, rt := range threads {
		out = append(out, ThreadDTOFrom(rt, projectID, repoID))
	}
	return out
}
