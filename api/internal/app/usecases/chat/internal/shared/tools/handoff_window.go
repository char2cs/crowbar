package tools

import "fmt"

// HandoffWindow trims an oldest-first turn list for the handoff document a provider is spawned with:
// the row at pin (the chat's first ask, or -1) leads, then the most recent turns, so a long chat keeps
// what it is for and what it is doing now. The note between them is read cold, so it names the tool.
func HandoffWindow[T any](
	chatID string,
	rows []T,
	pin int,
) (head []T, note string, recent []T) {
	total := len(rows)
	if total <= defaultChatLogTurns {
		return nil, "", rows
	}
	size := defaultChatLogTurns
	if pin >= 0 && pin <= total-size {
		head, size = rows[pin:pin+1], size-1
	}
	w := recentWindow(total, 0, 0, size, maxChatLogTurns)
	note = fmt.Sprintf(
		"[%d earlier turns are omitted here; below are the most recent %d of %d. "+
			"Call get_chat_log with chatId=%q to read them.]\n\n",
		w.start-len(head), w.end-w.start, total, chatID,
	)
	return head, note, rows[w.start:w.end]
}
