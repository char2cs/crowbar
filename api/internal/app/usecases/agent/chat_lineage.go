package agent

import "context"

// ChatLineage answers "what does this chat read" at spawn time: the chat
// ancestors, nearest parent first, a thread inherits context from.
type ChatLineage interface {
	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)
}
