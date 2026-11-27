package domain

import "time"

const (
	SurfaceChat     = "chat"
	SurfaceTerminal = "terminal"
)

func KnownSurface(s string) bool {
	return s == "" || s == SurfaceChat || s == SurfaceTerminal
}

type Chat struct {
	ID            string    `json:"id"`
	Type          ChatType  `json:"type"`
	WorkspaceID   string    `json:"workspaceId"`
	OwnsWorkspace bool      `json:"ownsWorkspace,omitempty"`
	Title         string    `json:"title"`
	TitleLocked   bool      `json:"titleLocked"`
	CreatedAt     time.Time `json:"createdAt"`

	RepoID string `json:"repoId,omitempty"`

	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`

	PermissionLevel string `json:"permissionLevel,omitempty"`

	PermissionLevelExplicit bool `json:"permissionLevelExplicit,omitempty"`

	Surface string `json:"surface,omitempty"`

	ProviderID string `json:"providerId,omitempty"`

	ParentID string `json:"-"`
	Order    int    `json:"-"`
}

type ChatSelection struct {
	Model  string
	Effort string
}
