package render

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RenderChatLog renders turns as the ledger's "<speaker>: <text>" blocks separated by a blank line, each
// body capped at MaxTurnBodyChars. Bodies are indented because another agent reads this: only a real turn
// header may start a line, and the separator stays the only truly empty one.
func RenderChatLog(turns []ChatTurn) string {
	var b strings.Builder
	for _, t := range turns {
		fmt.Fprintf(&b, "%s: %s\n\n",
			t.Speaker, indentBody(truncateBody(t.Body, MaxTurnBodyChars), "  "))
	}
	return b.String()
}

// RenderWorkspaces emits one header line per visible workspace, in the order given ("* " marks the
// caller's), followed by its chats indented so a free-text title can never pass for a header.
func RenderWorkspaces(
	caller domain.Workspace,
	visible []domain.Workspace,
	chats map[string][]domain.Chat,
) string {
	if len(visible) == 0 {
		return "No visible workspaces."
	}
	var b strings.Builder
	for _, w := range visible {
		if w.ID == caller.ID {
			b.WriteString("* ")
		}
		b.WriteString(w.ID)
		b.WriteString("\n")
		for _, chat := range chats[w.ID] {
			title := chat.Title
			if title == "" {
				title = "(untitled)"
			}
			// A title comes from a model via set_chat_title; folding every line-break shape keeps it
			// from forging a workspace header.
			title = strings.ReplaceAll(normalizeBreaks(title), "\n", " ")
			fmt.Fprintf(&b, "    %s  %s\n", chat.ID, title)
		}
	}
	return b.String()
}
