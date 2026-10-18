package chat

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// publishLedgerRows tells a chat's clients about every prompt the ledger
// settled. It runs under the chat's ledger writer; snapshots.Get takes the owner
// lock and may read stores beneath it, which is the sanctioned lock order.
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
			slog.Warn("agent: publish settled prompt: chat snapshot",
				"chat_id", chatID, "request_id", body.RequestID, "err", err)
			continue
		}
		feed.PromptSettled(chatID, snap.Chat.WorkspaceID, body.RequestID, body.Consumed)
	}
}
