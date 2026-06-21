package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ThreadReplyDTO is the wire shape of a reply on a review thread (00 §5.5): its
// id, the parent thread, the body, the author, the agent flag, and the creation
// timestamp.
type ThreadReplyDTO struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"threadId"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	IsAgent   bool      `json:"isAgent"`
	CreatedAt time.Time `json:"createdAt"`
}

// ThreadDTO is the wire shape of a workspace-scoped review thread (00 §5.5): the
// hierarchical entity ids, the anchored file location, the root comment, the
// resolution flag, and the ordered reply set. Replies is always a non-nil slice
// so the envelope carries [] rather than null when the thread has no replies.
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
	Body        string           `json:"body"`
	Author      string           `json:"author"`
	IsAgent     bool             `json:"isAgent"`
	Resolved    bool             `json:"resolved"`
	CreatedAt   time.Time        `json:"createdAt"`
	Replies     []ThreadReplyDTO `json:"replies"`
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
	body := ""
	author := ""
	rootIsAgent := false
	replies := make([]ThreadReplyDTO, 0, len(rt.Messages))
	for i, msg := range rt.Messages {
		if i == 0 {
			body = msg.Body
			author = msg.Author
			rootIsAgent = msg.IsAgent
			continue
		}
		replies = append(replies, ThreadReplyDTO{
			ID:        msg.ID,
			ThreadID:  rt.ID,
			Body:      msg.Body,
			Author:    msg.Author,
			IsAgent:   msg.IsAgent,
			CreatedAt: msg.CreatedAt,
		})
	}
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
		Body:        body,
		Author:      author,
		IsAgent:     rootIsAgent,
		Resolved:    rt.IsResolved(),
		CreatedAt:   rt.CreatedAt,
		Replies:     replies,
	}
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
