package domain

// GitStatus is the full working-tree status for a workspace (04 §3).
// Pushed directly on the Git topic (Class B) by the file watcher and git write
// usecases.
type GitStatus struct {
	Branch string    `json:"branch"`
	Ahead  int       `json:"ahead"`
	Behind int       `json:"behind"`
	Files  []GitFile `json:"files"`
}
