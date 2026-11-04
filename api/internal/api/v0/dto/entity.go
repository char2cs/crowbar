package dto

import "time"

// ChatEntityDTO is a chat as the projects scope carries it: the chat routes'
// DTO and the project and repo it is listed under ("" for a project home).
// Its worktree, and whether it owns it, are its workspace entity's.
type ChatEntityDTO struct {
	AgentChatDTO
	ProjectID string `json:"projectId"`
	RepoID    string `json:"repoId"`
}

// ChatEntityFrom is d listed under projectID and repoID.
func ChatEntityFrom(d AgentChatDTO, projectID, repoID string) ChatEntityDTO {
	d.Worktree = nil
	return ChatEntityDTO{AgentChatDTO: d, ProjectID: projectID, RepoID: repoID}
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

// PresenceDTO is how many connections view a chat, the client's own among them.
type PresenceDTO struct {
	Viewers int `json:"viewers"`
}
