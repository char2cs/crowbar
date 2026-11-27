package domain

type AgentModelManifestFetch struct {
	ID      string `gorm:"primaryKey"`
	Enabled bool
}

const ModelManifestFetchKey = "default"
