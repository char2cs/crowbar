package domain

// ChatType marks a row's kind in the sidebar forest: chat (conversation), branch
// (protected branch), folder (folder), or workflow. v0 only ever writes "chat";
// "workflow" is a forward-compat marker with no v0 writer (01 §2).
type ChatType string

const (
	ChatTypeChat     ChatType = "chat"
	ChatTypeBranch   ChatType = "branch"
	ChatTypeFolder   ChatType = "folder"
	ChatTypeWorkflow ChatType = "workflow"
)
