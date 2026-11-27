package render

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

const threadHeader = "id  file:lines  side  state  messages\n"

func RenderThreads(threads []domain.ReviewThread) string {
	if len(threads) == 0 {
		return "No review threads."
	}
	var b strings.Builder
	b.WriteString(threadHeader)
	for _, t := range threads {
		writeThread(&b, t, MaxThreadMessages)
	}
	return b.String()
}

func RenderThread(t domain.ReviewThread) string {
	var b strings.Builder
	b.WriteString(threadHeader)
	writeThread(&b, t, AllThreadMessages)
	return b.String()
}

func writeThread(
	b *strings.Builder,
	t domain.ReviewThread,
	messageCap int,
) {
	state := "unresolved"
	if t.IsResolved() {
		state = "resolved"
	}
	where, side := fmt.Sprintf("%s:%d-%d", t.FilePath, t.StartLine, t.EndLine), string(t.Side)
	if t.FilePath == "" {
		where, side = "general comment", "-"
	}
	fmt.Fprintf(b, "%s  %s  %s  %s  %d\n", t.ID, where, side, state, len(t.Messages))
	shown, elided := cappedMessages(t.Messages, messageCap)
	if elided > 0 {
		fmt.Fprintf(b,
			"    ... %d middle replies not shown (root + %d most recent below); "+
				"call list_review_threads with threadId=%s for every message\n",
			elided, messageCap-1, t.ID)
	}
	for _, m := range shown {
		author := m.Author
		if author == "" && !m.IsAgent {
			author = "user"
		}
		if m.IsAgent {
			author += " (agent)"
		}
		body := truncateBody(m.Body, MaxMessageBodyChars)
		fmt.Fprintf(b, "    %s: %s\n", author, indentBody(body, "      "))
	}
}

func indentBody(body, indent string) string {
	return strings.ReplaceAll(normalizeBreaks(body), "\n", "\n"+indent)
}

func normalizeBreaks(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func truncateBody(body string, max int) string {
	if len(body) <= max {
		return body
	}
	r := []rune(body)
	if len(r) <= max {
		return body
	}
	return fmt.Sprintf("%s… [shortened; %d characters not shown]", string(r[:max]), len(r)-max)
}

func cappedMessages(
	messages []domain.ReviewMessage,
	messageCap int,
) ([]domain.ReviewMessage, int) {
	if messageCap <= AllThreadMessages || len(messages) <= messageCap {
		return messages, 0
	}
	tail := messages[len(messages)-(messageCap-1):]
	kept := make([]domain.ReviewMessage, 0, messageCap)
	kept = append(kept, messages[0])
	kept = append(kept, tail...)
	return kept, len(messages) - len(kept)
}
