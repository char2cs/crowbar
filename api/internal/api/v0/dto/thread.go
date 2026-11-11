package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ThreadReplyDTO is the wire shape of a reply on a review thread. ProviderID and ChatID
// attribute an agent reply to its CLI and chat; absent on human and unattributed agent
// replies, where a client falls back to its generic rendering.
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

// ThreadDTO is a workspace-scoped review thread with its root message flattened on
// (MessageID, Body, attribution) so the root is editable like a reply. Replies is
// never nil; Deleted marks the tombstone frame broadcast when a thread is forgotten.
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

// ThreadDTOFrom converts a ReviewThread into ThreadDTO: its first message is the root,
// the rest become replies. projectID and repoID come from the caller, since the
// aggregate carries only its WsID.
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

// rootMessage is the thread's first message, or the zero message when it has
// none — which flattens to the empty root fields a message-less thread has always
// serialised, rather than panicking on an empty aggregate.
func rootMessage(
	rt domain.ReviewThread,
) domain.ReviewMessage {
	if len(rt.Messages) == 0 {
		return domain.ReviewMessage{}
	}
	return rt.Messages[0]
}

// threadReplies converts every message AFTER the root, in order, always
// returning a non-nil slice so the envelope carries [] rather than null.
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

// ThreadDTOList converts a slice of ReviewThread aggregates into wire ThreadDTOs
// sharing the same project/repo scope, returning a non-nil slice so the envelope
// carries [] rather than null when the workspace has no threads.
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
