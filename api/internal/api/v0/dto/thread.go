package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ThreadReplyDTO is the wire shape of a reply on a review thread (00 §5.5): its
// id, the parent thread, the body, the author, the agent flag, the optional
// agent attribution, and the creation timestamp.
//
// ProviderID and ChatID name the vendor CLI that wrote the reply and the chat it
// came out of, so the review UI can show which agent said what and offer a way
// back to that conversation. Both are omitempty and both are absent for a human
// reply and for every agent reply predating attribution — a client must fall back
// to its generic agent rendering rather than treating an empty id as a provider.
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

// ThreadDTO is the wire shape of a workspace-scoped review thread (00 §5.5): the
// hierarchical entity ids, the anchored file location, the root comment, the
// resolution flag, and the ordered reply set. Replies is always a non-nil slice
// so the envelope carries [] rather than null when the thread has no replies.
//
// MessageID is the real id of the root comment (Messages[0]). It is surfaced so
// the client can target the root for edit by the same /messages/:messageId route
// that addresses replies. Deleted marks a tombstone frame broadcast after a
// thread is forgotten, so subscribed clients can drop it from their store.
//
// ProviderID and ChatID belong to the ROOT message, alongside its body, author
// and agent flag — the root is flattened onto the thread rather than appearing in
// Replies, so without them here an agent-opened thread would render attributed on
// every reply and anonymous on the finding itself. They carry the same meaning
// and the same absent-by-default contract as ThreadReplyDTO's.
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

// ThreadDTOFrom converts a ReviewThread aggregate into the workspace-scoped wire
// ThreadDTO (00 §5.5). The ReviewThread carries only its WsID, so the owning
// projectID and repoID are supplied by the caller from the request path. The
// thread's first message is the root comment (its body/author surface at the top
// level); every later message becomes an ordered reply. Replies is always a
// non-nil slice so the envelope carries [] rather than null.
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
