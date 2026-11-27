package domain

type AgentPermissionDefault struct {
	ID    string `gorm:"primaryKey"`
	Level string
}

const DefaultPermissionLevelKey = "default"

func PermissionLevels() []string { return []string{"guarded", "trusted", "full-auto"} }
