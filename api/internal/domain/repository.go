package domain

type Repository struct {
	ID            string `gorm:"primaryKey" json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	MainBranch    string `json:"mainBranch,omitempty"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
	AvatarHasIcon bool   `json:"avatarHasIcon"`
	AvatarEmoji   string `json:"avatarEmoji,omitempty"`
	RemoteURL     string `json:"remoteUrl,omitempty"`
}

func (Repository) TableName() string {
	return "repositories"
}
