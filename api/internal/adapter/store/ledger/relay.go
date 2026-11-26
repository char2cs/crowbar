package ledger

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// RelayOwner answers which chat a relay id was bound in. It only locates the chat:
// whether the id still names a bound session is that chat's own State to say.
func (s *Store) RelayOwner(ctx context.Context, relayID string) (chatID string, ok bool, err error) {
	var owners []string
	err = s.db.WithContext(ctx).Raw(
		`SELECT chat_id FROM chat_ledger_relays WHERE relay_id = ?`, relayID,
	).Scan(&owners).Error
	if err != nil {
		return "", false, fmt.Errorf("ledger: relay owner: %w", err)
	}
	if len(owners) == 0 {
		return "", false, nil
	}
	return owners[0], true, nil
}

// indexRelay keeps the relay index with a binding: the first chat to bind an id
// holds it, and only a move hands it on, as the session itself moves.
func indexRelay(tx *gorm.DB, chatID string, body ledger.SessionBound) error {
	if body.RelayID == "" {
		return nil
	}
	err := tx.Exec(`INSERT INTO chat_ledger_relays (relay_id, chat_id) VALUES (?, ?)
		ON CONFLICT(relay_id) DO NOTHING`, body.RelayID, chatID).Error
	if err != nil {
		return fmt.Errorf("relay index: %w", err)
	}
	if body.Cause != ledger.CauseMoved {
		return nil
	}
	err = tx.Exec(`UPDATE chat_ledger_relays SET chat_id = ? WHERE relay_id = ? AND chat_id = ?`,
		chatID, body.RelayID, body.From).Error
	if err != nil {
		return fmt.Errorf("relay index: pass on: %w", err)
	}
	return nil
}
