package domain

// Repository is a git repo imported under a Project (00 §5.2).
type Repository struct {
	ID            string `gorm:"primaryKey" json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
}

func (Repository) TableName() string {
	return "repositories"
}
