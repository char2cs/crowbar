package tools

import "fmt"

// RecentHandoffWindow trims a chronologically-ordered (oldest first) turn list
// to the same "how much counts as recent" get_chat_log's own page already
// uses, for the handoff document a provider is spawned or resumed with.
//
// A short chat is returned unchanged, note included, so a gap that never
// exceeds the cap costs nothing beyond what it already cost — no note text,
// same slice. Only a chat that actually needed trimming pays for saying so.
//
// The note is not recentNote: that one answers a tool call the model already
// made with the chat id in hand. This document is injected cold, before any
// call at all, so the note has to name get_chat_log AND chatID itself or a
// model reading it has no way to act on "there is more".
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
