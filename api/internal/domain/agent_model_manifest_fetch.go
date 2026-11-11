package domain

// AgentModelManifestFetch is the singleton row (see ModelManifestFetchKey) saying
// whether Crowbar may refresh a model.manifest: catalogue over the network. Off falls
// back to the disk cache, then the embedded bundle, never an empty catalogue.
type AgentModelManifestFetch struct {
	ID      string `gorm:"primaryKey"`
	Enabled bool
}

// ModelManifestFetchKey is the fixed primary key AgentModelManifestFetch is
// always saved and loaded under.
const ModelManifestFetchKey = "default"
