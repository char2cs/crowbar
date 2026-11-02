package dto

import "time"

// ChatEntityDTO is a chat as the projects scope carries it: the DTO the chat
// routes answer with (its worktree left to the workspace entity), the project
// and repo it is listed under ("" for a project home), and whether it owns its
// workspace's worktree.
type ChatEntityDTO struct {
	AgentChatDTO
	ProjectID    string `json:"projectId"`
	RepoID       string `json:"repoId"`
	OwnsWorktree bool   `json:"ownsWorktree"`
}

// ChatEntityFrom is d listed under projectID and repoID.
func ChatEntityFrom(d AgentChatDTO, projectID, repoID string, ownsWorktree bool) ChatEntityDTO {
	d.Worktree = nil
	return ChatEntityDTO{AgentChatDTO: d, ProjectID: projectID, RepoID: repoID, OwnsWorktree: ownsWorktree}
}

// FolderEntityDTO is a sidebar folder as the projects scope carries it: what
// the folder routes answer with, and the scope those leave to the URL.
type FolderEntityDTO struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	RepoID    string    `json:"repoId"`
	ParentID  string    `json:"parentId"`
	Name      string    `json:"name"`
	Order     int       `json:"order"`
	CreatedAt time.Time `json:"createdAt"`
}
