package dto

import "time"

type ChatEntityDTO struct {
	AgentChatDTO
	ProjectID string `json:"projectId"`
	RepoID    string `json:"repoId"`
}

func ChatEntityFrom(d AgentChatDTO, projectID, repoID string) ChatEntityDTO {
	d.Worktree = nil
	return ChatEntityDTO{AgentChatDTO: d, ProjectID: projectID, RepoID: repoID}
}

type FolderEntityDTO struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	RepoID    string    `json:"repoId"`
	ParentID  string    `json:"parentId"`
	Name      string    `json:"name"`
	Order     int       `json:"order"`
	CreatedAt time.Time `json:"createdAt"`
}

type PresenceDTO struct {
	Viewers int `json:"viewers"`
}
