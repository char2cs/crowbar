package domain

// AgentModelManifestFetch is the one global row holding whether Crowbar may
// refresh a model.manifest: catalogue over the network. It is a singleton by
// convention (see ModelManifestFetchKey), not a table with one row per
// anything. Off falls back to the disk cache, then the embedded bundle —
// never an empty catalogue, and never a per-provider switch: which
// provider(s) use this source is the descriptor's own business.
type AgentModelManifestFetch struct {
	ID      string `gorm:"primaryKey"`
	Enabled bool
}

// ModelManifestFetchKey is the fixed primary key AgentModelManifestFetch is
// always saved and loaded under.
const ModelManifestFetchKey = "default"
