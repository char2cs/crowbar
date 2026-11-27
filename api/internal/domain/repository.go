package domain

type Repository struct {
	ID            string `gorm:"primaryKey" json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	PathSlug      string `json:"pathSlug,omitempty"`
	DefaultBranch string `json:"defaultBranch"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
	AvatarHasIcon bool   `json:"avatarHasIcon"`
	AvatarVersion int64  `json:"avatarVersion,omitempty"`
	AvatarEmoji   string `json:"avatarEmoji,omitempty"`
	RemoteURL     string `json:"remoteUrl,omitempty"`
	Deleting      bool   `json:"deleting,omitempty"`
}

func (Repository) TableName() string {
	return "repositories"
}
