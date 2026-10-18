package chat

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// publishLedgerRows tells a chat's clients about every prompt the ledger
// settled: the composer holding its text stops waiting for a turn. It runs
// under the chat's ledger writer, so it reads only the in-memory snapshot.
func (u *Usecase) publishLedgerRows(chatID string, rows []ledger.Row, _ ledger.State) {
	feed := u.feed.Load()
	if feed == nil || feed.PromptSettled == nil {
		return
	}
	for _, r := range rows {
		if r.Kind != ledger.KindPromptSettled {
			continue
		}
		body, err := ledger.DecodeBody[ledger.PromptSettled](r)
		if err != nil {
			continue
		}
		snap, err := u.snapshots.Get(context.Background(), chatID)
		if err != nil {
			slog.Warn("agent: publish settled prompt: chat snapshot", "chat_id", chatID, "err", err)
			return
		}
		feed.PromptSettled(chatID, snap.Chat.WorkspaceID, body.RequestID, body.Consumed)
	}
}
