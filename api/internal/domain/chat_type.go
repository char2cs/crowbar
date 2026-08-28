package domain

// ChatType marks a chat as an ordinary conversation or a workflow. v0 only ever
// writes "chat"; "workflow" is a forward-compat marker with no v0 writer (01 §2).
type ChatType string

const (
	ChatTypeChat     ChatType = "chat"
	ChatTypeBranch   ChatType = "branch"
	ChatTypeFolder   ChatType = "folder"
	ChatTypeWorkflow ChatType = "workflow"
)
