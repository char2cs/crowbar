package render

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// threadHeader names the columns of an anchor row. Keys appear once here, not
// per row, which is what makes this cheaper than JSON or YAML for the same data.
const threadHeader = "id  file:lines  side  state  messages\n"

// RenderThreads emits one anchor row per thread with the capped set of its
// messages, each on its own indented line. It is the LIST view: many threads,
// each summarised.
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

// RenderThread is the single-thread view: every message, so the list view's elided middle is reachable.
// Bodies are still capped at MaxMessageBodyChars.
func RenderThread(t domain.ReviewThread) string {
	var b strings.Builder
	b.WriteString(threadHeader)
	writeThread(&b, t, AllThreadMessages)
	return b.String()
}

// writeThread emits one thread's anchor row and up to messageCap messages (AllThreadMessages keeps all).
// Bodies are never inlined into a row, so markdown cannot corrupt the structure.
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
		// A general comment has no diff location, so coordinates would read as a
		// real ":0-0" anchor.
		where, side = "general comment", "-"
	}
	fmt.Fprintf(b, "%s  %s  %s  %s  %d\n", t.ID, where, side, state, len(t.Messages))
	// The anchor row reports the true message count, so the elision below is visible.
	shown, elided := cappedMessages(t.Messages, messageCap)
	if elided > 0 {
		// Not in "author: body" shape, so it cannot pass for a message; it names the argument that
		// fetches the rest.
		fmt.Fprintf(b,
			"    ... %d middle replies not shown (root + %d most recent below); "+
				"call list_review_threads with threadId=%s for every message\n",
			elided, messageCap-1, t.ID)
	}
	for _, m := range shown {
		author := m.Author
		// A human reply is stored with an empty author; only agent messages carry one.
		if author == "" && !m.IsAgent {
			author = "user"
		}
		if m.IsAgent {
			author += " (agent)"
		}
		// Truncate before indenting so the marker counts the body's own runes. The indent sits past the
		// message indent: another agent reads this, and a body line must never pass for a row or message.
		body := truncateBody(m.Body, MaxMessageBodyChars)
		fmt.Fprintf(b, "    %s: %s\n", author, indentBody(body, "      "))
	}
}

// indentBody re-indents every continuation line of a body so none can occupy a structural column. It
// normalises breaks first: a bare "\r" is a line break to a model too, and would otherwise forge a line.
func indentBody(body, indent string) string {
	return strings.ReplaceAll(normalizeBreaks(body), "\n", "\n"+indent)
}

// normalizeBreaks folds "\r\n" and a bare "\r" into "\n", without allocating when there is no "\r".
func normalizeBreaks(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// truncateBody shortens a body to max runes (never splitting a UTF-8 sequence) and says by how much, so
// a model does not read it as whole. The marker has no newline, so it stays inside the caller's indent.
func truncateBody(body string, max int) string {
	// Byte length is never less than rune count, so this settles the common case without allocating.
	if len(body) <= max {
		return body
	}
	r := []rune(body)
	if len(r) <= max {
		return body
	}
	return fmt.Sprintf("%s… [shortened; %d characters not shown]", string(r[:max]), len(r)-max)
}

// cappedMessages keeps the root (the finding) plus the most recent replies (its current state) and
// reports how many it dropped from the middle. AllThreadMessages keeps every message.
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
