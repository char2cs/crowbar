// Package types holds the shared domain types for the provider engine.
// Placed in a sub-package to avoid import cycles between the top-level
// provider package and its provider implementations.
package types

// PRInfo is the resolved pull-request state for a branch.
type PRInfo struct {
	Number       int    `json:"number"`
	Status       string `json:"status"` // "open" | "merged" | "closed"
	URL          string `json:"url"`
	Title        string `json:"title"`
	TargetBranch string `json:"targetBranch"`
}

// ProviderState is the full snapshot returned by a poll.
type ProviderState struct {
	Protected bool    `json:"protected"`
	PR        *PRInfo `json:"pr,omitempty"`
}

// ProviderCapability describes what the engine can do for a repo.
type ProviderCapability struct {
	Kind    string `json:"kind"`    // "github" | "gitlab" | "none"
	Enabled bool   `json:"enabled"` // false if CLI absent/unauthed
}
