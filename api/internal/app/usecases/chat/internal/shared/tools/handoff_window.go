package tools

import "fmt"

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
