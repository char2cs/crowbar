package render

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func RenderChatLog(turns []ChatTurn) string {
	var b strings.Builder
	for _, t := range turns {
		fmt.Fprintf(&b, "%s: %s\n\n",
			t.Speaker, indentBody(truncateBody(t.Body, MaxTurnBodyChars), "  "))
	}
	return b.String()
}

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
			title = strings.ReplaceAll(normalizeBreaks(title), "\n", " ")
			fmt.Fprintf(&b, "    %s  %s\n", chat.ID, title)
		}
	}
	return b.String()
}
