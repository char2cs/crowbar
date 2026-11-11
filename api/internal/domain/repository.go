package domain

// Repository carries a repo's own identity and its git-adjacent facts. Its sidebar
// position is not here: it lives in the project's tree.Tree.
type Repository struct {
	ID        string `gorm:"primaryKey" json:"id"`
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	// PathSlug is the repository's immutable on-disk identity, seeded at import from its
	// path or remote, never the renameable Name, so renaming cannot split its worktree tree.
	// Older rows carry "", and the slug chain (remote, PathSlug, Name) falls back to Name.
	PathSlug      string `json:"pathSlug,omitempty"`
	DefaultBranch string `json:"defaultBranch"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
	AvatarHasIcon bool   `json:"avatarHasIcon"`
	// AvatarVersion increments every time the on-disk icon bytes change. It is
	// threaded into the icon proxy URL (?v=N) so clients refetch the image when
	// the bytes change behind an otherwise-stable URL.
	AvatarVersion int64  `json:"avatarVersion,omitempty"`
	AvatarEmoji   string `json:"avatarEmoji,omitempty"`
	RemoteURL     string `json:"remoteUrl,omitempty"`
	// Deleting is the durable intent of a delete, written before any of the
	// teardown runs; it is one-way, and boot re-drives every row carrying it.
	Deleting bool `json:"deleting,omitempty"`
}

func (Repository) TableName() string {
	return "repositories"
}
