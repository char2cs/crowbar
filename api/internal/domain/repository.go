package domain

type Repository struct {
	ID        string `gorm:"primaryKey" json:"id"`
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	// PathSlug is the repository's IMMUTABLE on-disk identity, seeded once at
	// import from the repo's PATH (or its remote) and never from the display
	// Name. Every managed worktree is derived under
	// <home>/projects/<project>/<slug>/<branch>, so a slug that tracked the
	// renameable Name would fork the repo's tree in two the moment a user
	// renamed it: new workspaces would land under the new slug while the
	// existing ones stayed under the old, and the sibling scan that rejects a
	// case-only path clash would read an empty directory and pass every time.
	//
	// Rows written before this field existed carry "", which the slug chain
	// (remote → PathSlug → Name) degrades to the previous name-based behaviour.
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
}

func (Repository) TableName() string {
	return "repositories"
}
