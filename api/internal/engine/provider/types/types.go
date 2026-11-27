package types

type PRInfo struct {
	Number       int    `json:"number"`
	Status       string `json:"status"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	TargetBranch string `json:"targetBranch"`
}

type PRLink struct {
	Head   string `json:"head"`
	Base   string `json:"base"`
	Number int    `json:"number"`
	Status string `json:"status"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

type ProviderState struct {
	Protected bool    `json:"protected"`
	PR        *PRInfo `json:"pr,omitempty"`
}

type ProviderCapability struct {
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
}
