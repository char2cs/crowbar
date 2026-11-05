package domain

// ChatType marks a row's kind: "chat" is the only type minted; "workflow" has no
// writer yet. ChatTypeBranch and ChatTypeFolder are kept only so reads can recognise
// legacy rows on disk: folders are tree.Folder rows and positions live in the tree.
type ChatType string

const (
	ChatTypeChat     ChatType = "chat"
	ChatTypeBranch   ChatType = "branch"
	ChatTypeFolder   ChatType = "folder"
	ChatTypeWorkflow ChatType = "workflow"
)

// IsChat reports whether c is an ordinary conversation. Create refuses a
// row without a valid Type (commands.validChatType), so there is no untyped
// row to normalise.
func (c Chat) IsChat() bool {
	return c.Type == ChatTypeChat
}
