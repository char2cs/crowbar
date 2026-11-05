package tools

import "fmt"

// RecentHandoffWindow trims an oldest-first turn list to get_chat_log's default recent window for the
// handoff document a provider is spawned with; a short chat comes back unchanged with no note. The note
// names get_chat_log and chatID because the document is read cold, before the model has made any call.
func RecentHandoffWindow[T any](
	chatID string,
	rows []T,
) ([]T, string) {
	total := len(rows)
	if total <= defaultChatLogTurns {
		return rows, ""
	}
	w := recentWindow(total, 0, 0, defaultChatLogTurns, maxChatLogTurns)
	note := fmt.Sprintf(
		"Showing the most recent %d of %d turns in this chat; %d earlier "+
			"turns are omitted. Call get_chat_log with chatId=%q to read them.\n\n",
		w.end-w.start, total, w.start, chatID,
	)
	return rows[w.start:w.end], note
}
