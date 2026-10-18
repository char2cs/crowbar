package chat

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// publishLedgerRows tells a chat's clients about every prompt the ledger
// settled. It runs under the chat's ledger writer, so it reads the snapshot
// through Held: Get may read the ledger, and would wait on that writer.
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
		snap, err := u.snapshots.Held(context.Background(), chatID)
		if err != nil {
			slog.Warn("agent: publish settled prompt: chat snapshot",
				"chat_id", chatID, "request_id", body.RequestID, "err", err)
			continue
		}
		feed.PromptSettled(chatID, snap.Chat.WorkspaceID, body.RequestID, body.Consumed)
	}
}
