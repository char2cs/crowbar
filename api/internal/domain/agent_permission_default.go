package domain

// AgentPermissionDefault is the one global row holding the permission level a
// new chat is seeded with. It is a singleton by convention (see
// DefaultPermissionLevelKey), not a table with one row per anything.
type AgentPermissionDefault struct {
	ID    string `gorm:"primaryKey"`
	Level string
}

// DefaultPermissionLevelKey is the fixed primary key AgentPermissionDefault is
// always saved and loaded under.
const DefaultPermissionLevelKey = "default"

// PermissionLevels is Crowbar's own closed set of level names, least to most
// permissive; what each means to a provider is its descriptor's business.
func PermissionLevels() []string { return []string{"guarded", "trusted", "full-auto"} }
