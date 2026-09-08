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
	// Order is the repository's dense index within its project's sidebar section
	// — specifically, among whatever else shares its FolderID (below): every
	// other repo filed under the same project-home folder, or, for "", every
	// other root-level repo. It is not compared against a chat/folder row's own
	// Order at write time (the two live in different tables, densified
	// independently), only sorted alongside them client-side — the same loose
	// interleaving a workspace's own Order already has with a folder's.
	// AutoMigrate adds the column; rows written before it existed default to 0 and
	// fall back to the id tiebreak, which the first reorder replaces with a dense
	// sequence.
	Order int `json:"order"`
	// FolderID is the project-home folder this repo's entry is filed under, ""
	// for the project's home root. It is the repo's OWN placement within its
	// project's home tree — distinct from anything git: a repo still owns
	// exactly the same worktrees, branches and default workspace wherever its
	// entry happens to sit in the sidebar. Always a project-home folder (a
	// domain.Chat row with Type == ChatTypeFolder and RepoID == ""); never a
	// folder that lives inside a repo's own tree, which organises that repo's
	// branches and has nothing to do with where the repo itself is filed.
	FolderID string `json:"folderId,omitempty"`
}

func (Repository) TableName() string {
	return "repositories"
}
