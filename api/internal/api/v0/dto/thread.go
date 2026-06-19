package dto

import (
	"time"
)

// ThreadReplyDTO is the wire shape of a reply on a review thread (00 §5.5): its
// id, the parent thread, the body, the author, and the creation timestamp.
type ThreadReplyDTO struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"threadId"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
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
	Side        string           `json:"side"`
	Body        string           `json:"body"`
	Author      string           `json:"author"`
	Resolved    bool             `json:"resolved"`
	CreatedAt   time.Time        `json:"createdAt"`
	Replies     []ThreadReplyDTO `json:"replies"`
}
